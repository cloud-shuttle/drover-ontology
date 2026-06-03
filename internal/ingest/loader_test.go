package ingest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodebaseLoader_GitDelta(t *testing.T) {
	// 1. Create a temporary directory representing our codebase root
	tmpDir := t.TempDir()

	// 2. Initialize a git repo inside the temp dir
	cmdInit := exec.Command("git", "init")
	cmdInit.Dir = tmpDir
	if err := cmdInit.Run(); err != nil {
		t.Skip("git not available in test environment, skipping git delta tests")
	}

	// Configure git credentials to avoid warnings in tests
	cmdConfig1 := exec.Command("git", "config", "user.name", "Test User")
	cmdConfig1.Dir = tmpDir
	_ = cmdConfig1.Run()
	cmdConfig2 := exec.Command("git", "config", "user.email", "test@example.com")
	cmdConfig2.Dir = tmpDir
	_ = cmdConfig2.Run()

	// 3. Create a base file and commit it
	baseFile := filepath.Join(tmpDir, "go.mod")
	if err := os.WriteFile(baseFile, []byte("module test-module\n"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	cmdAdd := exec.Command("git", "add", "go.mod")
	cmdAdd.Dir = tmpDir
	_ = cmdAdd.Run()

	cmdCommit := exec.Command("git", "commit", "-m", "initial commit")
	cmdCommit.Dir = tmpDir
	_ = cmdCommit.Run()

	// 4. Create an untracked file (added)
	newFile := filepath.Join(tmpDir, "new-file.go")
	if err := os.WriteFile(newFile, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("failed to write new-file.go: %v", err)
	}

	// 5. Modify go.mod
	if err := os.WriteFile(baseFile, []byte("module test-module-modified\n"), 0644); err != nil {
		t.Fatalf("failed to modify go.mod: %v", err)
	}

	// 6. Initialize CodebaseLoader in delta mode
	loader := &CodebaseLoader{
		RootPath:    tmpDir,
		UseGitDelta: true,
	}

	if !loader.IsGitRepo() {
		t.Fatalf("Expected IsGitRepo to return true for initialized git repo")
	}

	addedOrModified, deleted, err := loader.GetGitChangedFiles()
	if err != nil {
		t.Fatalf("GetGitChangedFiles failed: %v", err)
	}

	// Verify we found go.mod and new-file.go
	foundMod := false
	foundNew := false
	for _, f := range addedOrModified {
		if f == "go.mod" {
			foundMod = true
		}
		if f == "new-file.go" {
			foundNew = true
		}
	}

	if !foundMod {
		t.Errorf("Expected modified 'go.mod' to be detected in Git changes")
	}
	if !foundNew {
		t.Errorf("Expected untracked 'new-file.go' to be detected in Git changes")
	}

	if len(deleted) != 0 {
		t.Errorf("Expected zero deleted files, got %d", len(deleted))
	}

	// Test the LoadGoCodebase output in delta mode
	contextStr, err := loader.LoadGoCodebase()
	if err != nil {
		t.Fatalf("LoadGoCodebase failed: %v", err)
	}

	if !strings.Contains(contextStr, "Mode: Git-Driven Incremental Ingestion (Delta Mode)") {
		t.Errorf("Expected context string to indicate Delta Mode was active")
	}
	if !strings.Contains(contextStr, "=== FILE: go.mod ===") {
		t.Errorf("Expected modified file 'go.mod' content to be included in delta context")
	}
	if !strings.Contains(contextStr, "=== FILE: new-file.go ===") {
		t.Errorf("Expected untracked file 'new-file.go' content to be included in delta context")
	}
}
