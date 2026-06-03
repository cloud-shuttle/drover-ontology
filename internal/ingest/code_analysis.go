package ingest

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// CodeAnalysis provides helpers that an RLM (or any recursive long-context ingester)
// can call from inside its REPL environment. They operate over the single large
// "context" string produced by CodebaseLoader so that the model can intelligently
// drill into files without the entire corpus being in the LLM prompt at once.
//
// All methods are safe to expose to a Yaegi interpreter or similar.
type CodeAnalysis struct {
	Context string // the full annotated output from CodebaseLoader
}

// ListFiles returns every file path that appeared with a "=== FILE: ... ===" marker.
func (a *CodeAnalysis) ListFiles() []string {
	var files []string
	lines := strings.Split(a.Context, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "=== FILE: ") {
			f := strings.TrimPrefix(line, "=== FILE: ")
			f = strings.TrimSpace(strings.TrimSuffix(f, " ==="))
			if f != "" {
				files = append(files, f)
			}
		}
	}
	return files
}

// GetFileContent returns the raw content of a single file by its relative path
// (as it appeared in the loader headers). This is the primary "zoom in" primitive
// the RLM will use before deciding to ParseGoFile or search further.
func (a *CodeAnalysis) GetFileContent(relPath string) (string, error) {
	marker := "=== FILE: " + relPath + " ==="
	start := strings.Index(a.Context, marker)
	if start == -1 {
		// Try without leading ./ if the caller passed a cleaned path
		alt := "=== FILE: " + strings.TrimPrefix(relPath, "./") + " ==="
		start = strings.Index(a.Context, alt)
	}
	if start == -1 {
		return "", fmt.Errorf("file not found in context: %s", relPath)
	}

	start += len(marker)
	// Find next file marker or end of context
	end := strings.Index(a.Context[start:], "=== FILE:")
	if end == -1 {
		end = len(a.Context)
	} else {
		end += start
	}

	content := strings.TrimSpace(a.Context[start:end])
	fmt.Printf("📄 Loaded file: %s (%d chars)\n", relPath, len(content))
	return content, nil
}

// FindReferences searches the entire loaded context for a symbol (type name,
// function, variable, package, term, etc.) and returns small surrounding snippets.
// This is the primitive for discovering "who calls / implements / references X".
func (a *CodeAnalysis) FindReferences(symbol string) string {
	if symbol == "" {
		return "FindReferences: empty symbol"
	}
	var results []string
	lines := strings.Split(a.Context, "\n")
	for i, line := range lines {
		if strings.Contains(line, symbol) {
			start := i - 1
			if start < 0 {
				start = 0
			}
			end := i + 2
			if end > len(lines) {
				end = len(lines)
			}
			snippet := strings.Join(lines[start:end], "\n")
			results = append(results, fmt.Sprintf("--- ~line %d ---\n%s", i+1, strings.TrimSpace(snippet)))
			if len(results) > 50 {
				results = append(results, "... (truncated; too many matches)")
				break
			}
		}
	}
	if len(results) == 0 {
		return fmt.Sprintf("No references found for symbol %q", symbol)
	}
	return fmt.Sprintf("References to %q (%d hits):\n%s", symbol, len(results), strings.Join(results, "\n\n"))
}

// ParseGoFile runs go/ast over a .go file (recovered via GetFileContent) and
// returns a structured summary of structs, interfaces, funcs, and methods.
// The RLM uses this to extract high-quality "Struct", "Interface", "Package"
// entities instead of treating source as undifferentiated text.
func (a *CodeAnalysis) ParseGoFile(relPath string) (string, error) {
	if !strings.HasSuffix(relPath, ".go") {
		return "", fmt.Errorf("ParseGoFile expects a .go file, got: %s", relPath)
	}

	content, err := a.GetFileContent(relPath)
	if err != nil {
		return "", err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, relPath, content, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("go/parser failed on %s: %w", relPath, err)
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("=== Parsed Go File: %s (package %s) ===\n\n", relPath, f.Name.Name))

	// Structs
	buf.WriteString("Structs:\n")
	ast.Inspect(f, func(n ast.Node) bool {
		if gen, ok := n.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			for _, spec := range gen.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					if _, isStruct := ts.Type.(*ast.StructType); isStruct {
						buf.WriteString(fmt.Sprintf("  - %s\n", ts.Name.Name))
					}
				}
			}
		}
		return true
	})

	// Interfaces
	buf.WriteString("\nInterfaces:\n")
	ast.Inspect(f, func(n ast.Node) bool {
		if gen, ok := n.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
			for _, spec := range gen.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					if _, isIface := ts.Type.(*ast.InterfaceType); isIface {
						buf.WriteString(fmt.Sprintf("  - %s\n", ts.Name.Name))
					}
				}
			}
		}
		return true
	})

	// Functions + methods
	buf.WriteString("\nFunctions / Methods:\n")
	ast.Inspect(f, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			recv := ""
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recv = " (method on " + exprString(fn.Recv.List[0].Type) + ")"
			}
			buf.WriteString(fmt.Sprintf("  - %s%s\n", fn.Name.Name, recv))
		}
		return true
	})

	// Top-level const/var (lightweight)
	buf.WriteString("\nTop-level declarations (const/var):\n")
	for _, decl := range f.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			if gen.Tok == token.CONST || gen.Tok == token.VAR {
				for _, spec := range gen.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							buf.WriteString(fmt.Sprintf("  - %s %s\n", gen.Tok.String(), name.Name))
						}
					}
				}
			}
		}
	}

	fmt.Printf("🔍 Parsed Go file: %s\n", relPath)
	return buf.String(), nil
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	default:
		return fmt.Sprintf("%T", e)
	}
}

// DroverSpecificHelpers are small sugar functions the RLM can call to recognize
// drover-ontology / drover-org conventions quickly (without having to re-learn
// the schema on every run).
func (a *CodeAnalysis) ListDroverEntityTypes() []string {
	return []string{
		"Term", "Concept", "Taxonomy", "Document", "Note", "Message", "Thread", "Policy",
		"Person", "Organization", "Project", "Goal", "Event", "Location",
		"Bead", "Ticket", "Plan", "Review", "Change",
		"Package", "Module", "Struct", "Interface", "Function", "Service", "Repository",
		"DomainConcept", "ADR", "Skill",
	}
}

// RecommendedDroverRelations returns the canonical set the model should prefer
// when calling DroverRelate (see drover_client.go). This list is intentionally
// the union of core-schema + drover-schema + taxonomy-relations.
func (a *CodeAnalysis) RecommendedDroverRelations() []string {
	return []string{
		"broader_term", "narrower_term", "tagged_with", "in_domain", "related_to", "governed_by",
		"part_of", "contains", "references", "covers", "teaches", "implements", "depends_on",
		"owns", "has_owner", "member_of", "serves", "approves", "reviewed_by", "tracks",
		"blocks",
	}
}

// LoadBeadsJSON is a convenience that tries to return the beads.json content
// (the drover work-tracking source of truth). The RLM can use it to seed
// Plan / Bead / Review entities with proper ids and status.
func (a *CodeAnalysis) LoadBeadsJSON() (string, error) {
	// Common locations (include both the snapshot .json and the line-oriented .jsonl export for RLM/ingest scenarios)
	candidates := []string{"beads.json", "beads.jsonl", "drover-org/beads.json", "drover-org/beads.jsonl"}
	for _, c := range candidates {
		if data, err := a.GetFileContent(c); err == nil {
			return data, nil
		}
	}
	// Fallback: search the context for any beads.json-ish content
	if idx := strings.Index(a.Context, `"beads":`); idx != -1 {
		// Return a generous window around it
		start := idx - 200
		if start < 0 {
			start = 0
		}
		end := idx + 4000
		if end > len(a.Context) {
			end = len(a.Context)
		}
		return a.Context[start:end], nil
	}
	return "", fmt.Errorf("beads.json not found in loaded context")
}

// ParseFileSymbols detects the file extension and runs an appropriate syntax scanner
// to identify class, structure, function definitions and yaml keys with line numbers.
// This supports language-agnostic code analysis (Go, JS/TS, Python, YAML) with high compression.
func (a *CodeAnalysis) ParseFileSymbols(relPath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(relPath))
	if ext == ".go" {
		return a.ParseGoFile(relPath)
	}

	content, err := a.GetFileContent(relPath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("=== Parsed Symbols: %s ===\n\n", relPath))

	switch ext {
	case ".js", ".jsx", ".ts", ".tsx":
		a.parseJSTSSymbols(content, &buf)
	case ".py":
		a.parsePythonSymbols(content, &buf)
	case ".yaml", ".yml":
		a.parseYAMLKeys(content, &buf)
	default:
		a.parseFallbackSymbols(content, &buf)
	}

	return buf.String(), nil
}

// parseJSTSSymbols extracts classes, functions, and arrow functions with line spans.
func (a *CodeAnalysis) parseJSTSSymbols(content string, buf *bytes.Buffer) {
	lines := strings.Split(content, "\n")

	buf.WriteString("Classes:\n")
	hasClasses := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "class ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				className := strings.Split(parts[1], "{")[0]
				className = strings.Split(className, "<")[0] // strip generics
				className = strings.TrimSpace(className)
				endLine := findBlockEnd(lines, i)
				buf.WriteString(fmt.Sprintf("  - %s [lines %d-%d]\n", className, i+1, endLine))
				hasClasses = true
			}
		}
	}
	if !hasClasses {
		buf.WriteString("  (none)\n")
	}

	buf.WriteString("\nFunctions / Methods:\n")
	hasFuncs := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "class ") {
			continue
		}

		isFunc := false
		var funcName string

		if strings.HasPrefix(trimmed, "function ") || strings.HasPrefix(trimmed, "export function ") || strings.HasPrefix(trimmed, "export default function ") {
			isFunc = true
			parts := strings.Fields(trimmed)
			for idx, part := range parts {
				if part == "function" && idx+1 < len(parts) {
					funcName = strings.Split(parts[idx+1], "(")[0]
					break
				}
			}
		} else if strings.HasPrefix(trimmed, "async function ") || strings.HasPrefix(trimmed, "export async function ") || strings.HasPrefix(trimmed, "export default async function ") {
			isFunc = true
			parts := strings.Fields(trimmed)
			for idx, part := range parts {
				if part == "function" && idx+1 < len(parts) {
					funcName = strings.Split(parts[idx+1], "(")[0]
					break
				}
			}
		}

		if !isFunc && (strings.Contains(trimmed, "=>") && (strings.HasPrefix(trimmed, "const ") || strings.HasPrefix(trimmed, "let ") || strings.HasPrefix(trimmed, "var ") || strings.HasPrefix(trimmed, "export const "))) {
			parts := strings.Fields(trimmed)
			for idx, part := range parts {
				if (part == "const" || part == "let" || part == "var") && idx+1 < len(parts) {
					funcName = parts[idx+1]
					isFunc = true
					break
				}
			}
		}

		if !isFunc && strings.Contains(trimmed, "{") && strings.Contains(trimmed, "(") && !strings.Contains(trimmed, "=") {
			idxParen := strings.Index(trimmed, "(")
			idxBrace := strings.Index(trimmed, "{")
			if idxParen > 0 && idxBrace > idxParen {
				namePart := strings.TrimSpace(trimmed[:idxParen])
				parts := strings.Fields(namePart)
				if len(parts) > 0 {
					possibleName := parts[len(parts)-1]
					keywords := map[string]bool{
						"if": true, "for": true, "while": true, "switch": true, "catch": true,
					}
					if !keywords[possibleName] && !strings.Contains(possibleName, "[") && !strings.Contains(possibleName, ".") {
						funcName = possibleName
						isFunc = true
					}
				}
			}
		}

		if isFunc && funcName != "" {
			funcName = strings.TrimSpace(funcName)
			if len(funcName) > 0 {
				endLine := findBlockEnd(lines, i)
				buf.WriteString(fmt.Sprintf("  - %s [lines %d-%d]\n", funcName, i+1, endLine))
				hasFuncs = true
			}
		}
	}
	if !hasFuncs {
		buf.WriteString("  (none)\n")
	}
}

// parsePythonSymbols extracts class and def structures from Python files.
func (a *CodeAnalysis) parsePythonSymbols(content string, buf *bytes.Buffer) {
	lines := strings.Split(content, "\n")

	buf.WriteString("Classes:\n")
	hasClasses := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "class ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				className := strings.Split(parts[1], ":")[0]
				className = strings.Split(className, "(")[0]
				className = strings.TrimSpace(className)
				endLine := findPythonBlockEnd(lines, i)
				buf.WriteString(fmt.Sprintf("  - %s [lines %d-%d]\n", className, i+1, endLine))
				hasClasses = true
			}
		}
	}
	if !hasClasses {
		buf.WriteString("  (none)\n")
	}

	buf.WriteString("\nFunctions / Methods:\n")
	hasFuncs := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "def ") {
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				funcName := strings.Split(parts[1], "(")[0]
				funcName = strings.TrimSpace(funcName)
				endLine := findPythonBlockEnd(lines, i)
				buf.WriteString(fmt.Sprintf("  - %s [lines %d-%d]\n", funcName, i+1, endLine))
				hasFuncs = true
			}
		}
	}
	if !hasFuncs {
		buf.WriteString("  (none)\n")
	}
}

// parseYAMLKeys extracts top-level keys from YAML files.
func (a *CodeAnalysis) parseYAMLKeys(content string, buf *bytes.Buffer) {
	lines := strings.Split(content, "\n")

	buf.WriteString("Top-level Keys:\n")
	hasKeys := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			key := strings.TrimSpace(parts[0])
			if len(key) > 0 && !strings.Contains(key, "{") && !strings.Contains(key, "[") {
				endLine := len(lines)
				for j := i + 1; j < len(lines); j++ {
					lineJ := lines[j]
					trimmedJ := strings.TrimSpace(lineJ)
					if trimmedJ == "" || strings.HasPrefix(trimmedJ, "#") {
						continue
					}
					if !strings.HasPrefix(lineJ, " ") && !strings.HasPrefix(lineJ, "\t") && strings.Contains(lineJ, ":") {
						partsJ := strings.SplitN(lineJ, ":", 2)
						keyJ := strings.TrimSpace(partsJ[0])
						if len(keyJ) > 0 && !strings.Contains(keyJ, "{") && !strings.Contains(keyJ, "[") {
							endLine = j
							break
						}
					}
				}
				buf.WriteString(fmt.Sprintf("  - %s [lines %d-%d]\n", key, i+1, endLine))
				hasKeys = true
			}
		}
	}
	if !hasKeys {
		buf.WriteString("  (none)\n")
	}
}

// parseFallbackSymbols parses unsupported structures using standard signature keywords.
func (a *CodeAnalysis) parseFallbackSymbols(content string, buf *bytes.Buffer) {
	lines := strings.Split(content, "\n")

	buf.WriteString("Identified Blocks:\n")
	hasBlocks := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		lower := strings.ToLower(trimmed)
		isMatch := false
		keyword := ""
		name := ""

		if strings.HasPrefix(lower, "class ") || strings.HasPrefix(lower, "struct ") || strings.HasPrefix(lower, "interface ") {
			isMatch = true
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				keyword = parts[0]
				name = strings.TrimSuffix(parts[1], "{")
				name = strings.TrimSuffix(name, ":")
			}
		} else if strings.HasPrefix(lower, "def ") || strings.HasPrefix(lower, "fn ") || strings.HasPrefix(lower, "func ") || strings.HasPrefix(lower, "function ") {
			isMatch = true
			parts := strings.Fields(trimmed)
			if len(parts) > 1 {
				keyword = parts[0]
				name = strings.Split(parts[1], "(")[0]
			}
		}

		if isMatch && name != "" {
			name = strings.TrimSpace(name)
			buf.WriteString(fmt.Sprintf("  - %s (%s) [line %d]\n", name, keyword, i+1))
			hasBlocks = true
		}
	}
	if !hasBlocks {
		buf.WriteString("  (none - fallback scan found no structural keywords)\n")
	}
}

// findBlockEnd parses matching curly braces in TS/JS files.
func findBlockEnd(lines []string, startLineIndex int) int {
	braceCount := 0
	started := false
	inMultiLineComment := false

	for i := startLineIndex; i < len(lines); i++ {
		line := lines[i]
		inSingleLineComment := false
		inStringDouble := false
		inStringSingle := false
		inStringBacktick := false

		for j := 0; j < len(line); j++ {
			char := line[j]

			if inSingleLineComment {
				break
			}
			if inMultiLineComment {
				if j+1 < len(line) && line[j] == '*' && line[j+1] == '/' {
					inMultiLineComment = false
					j++
				}
				continue
			}
			if !inStringDouble && !inStringSingle && !inStringBacktick {
				if j+1 < len(line) && line[j] == '/' && line[j+1] == '/' {
					inSingleLineComment = true
					j++
					continue
				}
				if j+1 < len(line) && line[j] == '/' && line[j+1] == '*' {
					inMultiLineComment = true
					j++
					continue
				}
			}

			if char == '"' && !inStringSingle && !inStringBacktick {
				inStringDouble = !inStringDouble
				continue
			}
			if char == '\'' && !inStringDouble && !inStringBacktick {
				inStringSingle = !inStringSingle
				continue
			}
			if char == '`' && !inStringDouble && !inStringSingle {
				inStringBacktick = !inStringBacktick
				continue
			}

			if inStringDouble || inStringSingle || inStringBacktick {
				continue
			}

			if char == '{' {
				braceCount++
				started = true
			} else if char == '}' {
				if started {
					braceCount--
					if braceCount <= 0 {
						return i + 1
					}
				}
			}
		}
	}
	return len(lines)
}

// findPythonBlockEnd locates python block boundaries based on indentation level.
func findPythonBlockEnd(lines []string, startLineIndex int) int {
	startLine := lines[startLineIndex]
	startIndent := getIndentLevel(startLine)

	for i := startLineIndex + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := getIndentLevel(line)
		if indent <= startIndent {
			return i
		}
	}
	return len(lines)
}

// getIndentLevel computes indentation weight.
func getIndentLevel(line string) int {
	indent := 0
	for _, char := range line {
		if char == ' ' {
			indent++
		} else if char == '\t' {
			indent += 4
		} else {
			break
		}
	}
	return indent
}
