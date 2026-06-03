package ingest

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CodebaseLoader walks a directory (typically a Go monorepo or drover-org checkout)
// and produces a single well-structured string suitable for RLM "context" variable.
//
// It is aware of drover-ontology layout and common monorepo noise so that the
// recursive model sees clean, high-signal input (go.mod first, key docs, Go sources,
// schema, taxonomy files, beads, etc.) without drowning in vendor or build artifacts.
type CodebaseLoader struct {
	RootPath string

	// MaxFileBytes caps individual file size before truncation (0 = no cap).
	MaxFileBytes int64

	// UseGitDelta enables Git-driven incremental ingestion (Delta Mode).
	UseGitDelta bool
}

// LoadGoCodebase returns the concatenated, header-annotated content of the relevant
// files under RootPath. The output format is designed so that simple string search
// (see code_analysis.go) can recover individual file contents, and so that an RLM
// can reason over "=== FILE: path ===" markers.
func (l *CodebaseLoader) LoadGoCodebase() (string, error) {
	if l.RootPath == "" {
		l.RootPath = "."
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = 512 * 1024 // 512KB default truncation threshold
	}

	var buf bytes.Buffer

	// Project overview tailored for drover-style monorepos
	buf.WriteString("=== PROJECT OVERVIEW ===\n")
	buf.WriteString(fmt.Sprintf("Root: %s\n", l.RootPath))
	buf.WriteString("Language: Go\n")
	if l.UseGitDelta && l.IsGitRepo() {
		buf.WriteString("Mode: Git-Driven Incremental Ingestion (Delta Mode)\n")
	} else {
		buf.WriteString("Mode: Full Codebase Walk\n")
	}
	buf.WriteString("Note: This loader is drover-ontology aware. It prioritizes go.mod, READMEs, Go sources, schema files, taxonomy definitions, beads.json, CONTEXT.md, SKILL.md, references/, and .ontology/ JSONL (for history introspection). It aggressively skips build artifacts, testdata, vendor, node_modules, and the SQLite DB itself.\n\n")

	// Always try to surface go.mod first (architecture + module identity)
	modPath := filepath.Join(l.RootPath, "go.mod")
	if data, err := os.ReadFile(modPath); err == nil {
		buf.WriteString("=== FILE: go.mod ===\n")
		buf.Write(data)
		buf.WriteString("\n\n")
	}

	if l.UseGitDelta && l.IsGitRepo() {
		addedOrModified, deleted, err := l.GetGitChangedFiles()
		if err == nil {
			// Print deleted files first so RLM can invalidate them
			for _, file := range deleted {
				buf.WriteString(fmt.Sprintf("=== DELETED FILE: %s ===\n\n", file))
			}
			// Print added or modified files
			for _, file := range addedOrModified {
				absPath := filepath.Join(l.RootPath, file)
				data, err := os.ReadFile(absPath)
				if err != nil {
					continue // skip unreadable
				}
				if int64(len(data)) > l.MaxFileBytes {
					trunc := append(data[:l.MaxFileBytes], []byte("\n\n... [file truncated by CodebaseLoader due to size]")...)
					data = trunc
				}
				buf.WriteString(fmt.Sprintf("=== FILE: %s ===\n", file))
				buf.Write(data)
				buf.WriteString("\n\n")
			}
			return buf.String(), nil
		}
		// Fallback to normal walk on error
	}

	// Walk
	err := filepath.WalkDir(l.RootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(l.RootPath, path)

		// Skip noisy directories entirely
		if shouldSkipDir(relPath, d) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if !shouldIncludeFile(relPath) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // unreadable -> skip silently
		}

		if int64(len(data)) > l.MaxFileBytes {
			trunc := append(data[:l.MaxFileBytes], []byte("\n\n... [file truncated by CodebaseLoader due to size]")...)
			data = trunc
		}

		buf.WriteString(fmt.Sprintf("=== FILE: %s ===\n", relPath))
		buf.Write(data)
		buf.WriteString("\n\n")
		return nil
	})
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

// IsGitRepo returns true if the RootPath is inside a valid git working tree.
func (l *CodebaseLoader) IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = l.RootPath
	err := cmd.Run()
	return err == nil
}

// GetGitChangedFiles runs git commands to identify modified/added/deleted/untracked files.
func (l *CodebaseLoader) GetGitChangedFiles() (addedOrModified []string, deleted []string, err error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = l.RootPath
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}
		status := line[:2]
		path := line[3:]

		// Handle quoted paths if git output contains quotes
		if strings.HasPrefix(path, "\"") && strings.HasSuffix(path, "\"") {
			path = path[1 : len(path)-1]
		}

		// Filter out ignored/noise files using simple rules
		isExcluded := false
		for _, s := range []string{".git", "vendor", "node_modules", "bin", "build", "dist", ".ontology/graph.db"} {
			if strings.Contains(path, s) {
				isExcluded = true
				break
			}
		}
		if isExcluded {
			continue
		}
		if !shouldIncludeFile(path) {
			continue
		}

		// Check if it's deleted
		if strings.Contains(status, "D") {
			deleted = append(deleted, path)
		} else {
			addedOrModified = append(addedOrModified, path)
		}
	}
	return addedOrModified, deleted, nil
}

func shouldSkipDir(relPath string, d fs.DirEntry) bool {
	if relPath == "." {
		return false
	}
	// Common noise + drover-ontology specific build/test/output dirs
	skip := []string{
		".git", ".hg", ".svn",
		"vendor", "node_modules", "bower_components",
		"bin", "build", "dist", "out", "tmp", "temp", ".cache",
		"coverage", "testdata", "__debug_bin", ".idea", ".vscode",
		// Web application build / cache directories
		".next", ".open-next", ".vercel", ".wrangler", ".serena", "playwright-report",
		// drover-ontology specifics
		"internal/ontology/testdata/fuzz",
		"internal/ontology/testdata/rapid",
		".ontology/graph.db", // the SQLite projection is rebuilt from JSONL; do not slurp the binary
		"snapshots", // we may want JSONL history but snapshots/ dirs are usually large binary-ish
	}
	for _, s := range skip {
		if strings.Contains(relPath, s) {
			return true
		}
	}
	// Skip hidden dirs at root level except .github (CI is often useful)
	if strings.HasPrefix(filepath.Base(relPath), ".") && !strings.Contains(relPath, ".github") {
		return true
	}
	return false
}

func shouldIncludeFile(relPath string) bool {
	ext := strings.ToLower(filepath.Ext(relPath))
	base := strings.ToLower(filepath.Base(relPath))

	includeExts := []string{
		".go", ".mod", ".sum",
		".js", ".jsx", ".ts", ".tsx",
		".md", ".markdown",
		".yaml", ".yml", ".json", ".toml",
		".proto", ".sql", ".sh",
		".txt", ".rst",
	}
	for _, e := range includeExts {
		if ext == e {
			return true
		}
	}

	// Important root or near-root files even without extension
	important := []string{
		"readme", "license", "dockerfile", "makefile", "justfile",
		"beads.json", "drover-schema", "core-schema",
		"context.md", "skill.md", "contributing.md", "architecture.md",
		"adr", // any adr file or dir
	}
	for _, f := range important {
		if strings.Contains(base, f) {
			return true
		}
	}

	// Always include the actual ontology log for historical introspection (it is text JSONL)
	if strings.HasSuffix(relPath, "graph.jsonl") {
		return true
	}
	// Include beads tracking files (both the current snapshot and the .jsonl export used for some RLM/ingest flows)
	if strings.HasSuffix(relPath, "beads.json") || strings.HasSuffix(relPath, "beads.jsonl") {
		return true
	}

	// Include schema files in references/ and memory/
	if strings.Contains(relPath, "schema.yaml") || strings.Contains(relPath, "schema.yml") {
		return true
	}

	return false
}
