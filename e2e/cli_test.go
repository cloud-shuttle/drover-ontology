package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// moduleRoot walks upward from the current working directory until it locates
// a go.mod file. This makes the E2E harness robust regardless of which
// subdirectory `go test` was invoked from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot determine working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod by walking up from %s", dir)
		}
		dir = parent
	}
}

// buildOntologyBinary builds the ontology CLI binary into a temp location
// and returns the absolute path to the executable.
// It is safe to call from multiple tests; each gets its own binary copy.
func buildOntologyBinary(t *testing.T) string {
	t.Helper()

	root := moduleRoot(t)
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "ontology")

	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/ontology")
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to build ontology binary for E2E: %v", err)
	}

	// Make executable (defensive on some filesystems)
	_ = os.Chmod(binPath, 0o755)
	return binPath
}

// cliResult captures the outcome of running the ontology binary.
type cliResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// runCLI executes the ontology binary with the given args, forcing it to use
// an isolated temp store under workDir. It always supplies --graph, --db and --schema
// so that E2E tests are hermetic and do not touch the developer's real memory/ontology store.
func runCLI(t *testing.T, bin, workDir string, args ...string) cliResult {
	t.Helper()

	graph := filepath.Join(workDir, "graph.jsonl")
	db := filepath.Join(workDir, "graph.db")

	// Use a real schema from the repo so that schema-aware commands behave consistently.
	// moduleRoot guarantees we get an absolute path even if the test binary's CWD is e2e/.
	root := moduleRoot(t)
	schema := filepath.Join(root, "references", "drover-schema.yaml")
	if _, err := os.Stat(schema); err != nil {
		t.Fatalf("cannot locate schema file for E2E at %s: %v", schema, err)
	}

	t.Logf("E2E runCLI: --graph=%s --db=%s (workDir=%s)", graph, db, workDir)

	fullArgs := []string{
		"--graph", graph,
		"--db", db,
		"--schema", schema,
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command(bin, fullArgs...)
	cmd.Dir = workDir // run with workDir as CWD (harmless for our flags)

	// Capture stdout and stderr separately for better diagnostics (especially fallback warnings)
	stdout, err := cmd.Output()
	var stderr []byte
	var exitCode int
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	outStr := string(stdout)
	if len(stderr) > 0 {
		outStr += "\n[stderr]\n" + string(stderr)
	}

	return cliResult{
		Stdout:   outStr,
		Stderr:   string(stderr),
		ExitCode: exitCode,
	}
}

// requireSuccess is a small assertion helper.
func requireSuccess(t *testing.T, res cliResult, msgAndArgs ...any) {
	t.Helper()
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d\noutput:\n%s", res.ExitCode, res.Stdout)
	}
}

// TestE2E_CreateGet exercises the most basic flow: build the binary,
// create an entity via the real CLI, then retrieve it.
func TestE2E_CreateGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI E2E in short mode")
	}

	bin := buildOntologyBinary(t)
	workDir := t.TempDir()

	// Create
	create := runCLI(t, bin, workDir,
		"create", "Bead", "e2e-bead-001",
		"-p", "title=First E2E Bead",
		"-p", "status=draft",
		"-p", "kind=task",
	)
	requireSuccess(t, create, "create should succeed")
	if !strings.Contains(create.Stdout, "✅ Created e2e-bead-001") {
		t.Fatalf("unexpected create output:\n%s", create.Stdout)
	}

	// Get
	get := runCLI(t, bin, workDir, "get", "e2e-bead-001")
	requireSuccess(t, get, "get should succeed")
	if !strings.Contains(get.Stdout, "e2e-bead-001") || !strings.Contains(get.Stdout, "First E2E Bead") {
		t.Fatalf("get did not return expected entity:\n%s", get.Stdout)
	}
}

// TestE2E_CreateRelateGet exercises create + relate + retrieval.
// We verify successful relate message and that a subsequent get/list still works.
func TestE2E_CreateRelateGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI E2E in short mode")
	}

	bin := buildOntologyBinary(t)
	workDir := t.TempDir()

	// Two creates (valid per schema: Bead implements Plan)
	runCLI(t, bin, workDir, "create", "Bead", "e2e-bead-002", "-p", "title=Child Bead,kind=task,status=open")
	runCLI(t, bin, workDir, "create", "Plan", "e2e-plan-001", "-p", "title=Parent Plan,kind=feature,status=draft")

	// Relate (valid direction per implements: from Bead/Ticket to Plan/Goal)
	relate := runCLI(t, bin, workDir, "relate", "e2e-bead-002", "implements", "e2e-plan-001")
	requireSuccess(t, relate, "relate should succeed")
	if !strings.Contains(relate.Stdout, "✅ Related e2e-bead-002 --[implements]--> e2e-plan-001") {
		t.Fatalf("unexpected relate output:\n%s", relate.Stdout)
	}

	// Get one of them (proves the store still works after relate)
	get := runCLI(t, bin, workDir, "get", "e2e-bead-002")
	requireSuccess(t, get, "get after relate should succeed")
	if !strings.Contains(get.Stdout, "e2e-bead-002") {
		t.Fatalf("get after relate failed:\n%s", get.Stdout)
	}

	// List to exercise another read path
	list := runCLI(t, bin, workDir, "list")
	requireSuccess(t, list, "list should succeed")
	if !strings.Contains(list.Stdout, "e2e-plan-001") || !strings.Contains(list.Stdout, "e2e-bead-002") {
		t.Fatalf("list did not show created entities:\n%s", list.Stdout)
	}
}

// TestE2E_Fsck exercises fsck after mutations (the command the user explicitly called out).
// We do a few creates/relates then run fsck and assert clean output.
func TestE2E_Fsck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI E2E in short mode")
	}

	bin := buildOntologyBinary(t)
	workDir := t.TempDir()

	// Seed some data (must be valid per the schema passed to CLI / loaded by Ontology)
	runCLI(t, bin, workDir, "create", "Task", "e2e-task-fsck", "-p", "title=FSck target,status=open")
	runCLI(t, bin, workDir, "create", "Plan", "e2e-plan-fsck", "-p", "title=FSck plan,kind=feature,status=draft")
	runCLI(t, bin, workDir, "create", "Review", "e2e-review-fsck", "-p", "kind=asdlc,decision=go,reviewer=test")
	runCLI(t, bin, workDir, "relate", "e2e-task-fsck", "depends_on", "e2e-plan-fsck")

	// Run fsck (default full check)
	fsck := runCLI(t, bin, workDir, "fsck")
	requireSuccess(t, fsck, "fsck should succeed")
	if !strings.Contains(fsck.Stdout, "✅ Full integrity check passed") {
		t.Fatalf("fsck did not report success:\n%s", fsck.Stdout)
	}

	// Also exercise the --check-invalidate flag (even though it currently does full check)
	fsckInv := runCLI(t, bin, workDir, "fsck", "--check-invalidate")
	requireSuccess(t, fsckInv, "fsck --check-invalidate should succeed")
	if !strings.Contains(fsckInv.Stdout, "integrity check") && !strings.Contains(fsckInv.Stdout, "passed") {
		t.Fatalf("fsck --check-invalidate output unexpected:\n%s", fsckInv.Stdout)
	}
}

// TestE2E_Delete exercises the real delete command (tombstone + invalidate).
// Verifies that after delete, the entity no longer appears in list by default.
func TestE2E_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI E2E in short mode")
	}

	bin := buildOntologyBinary(t)
	workDir := t.TempDir()

	// Create an entity
	create := runCLI(t, bin, workDir, "create", "Bead", "to-be-deleted", "-p", "title=doomed,kind=task,status=open")
	requireSuccess(t, create, "create should succeed")

	// Delete it with reason
	del := runCLI(t, bin, workDir, "delete", "to-be-deleted", "--reason", "test cleanup")
	requireSuccess(t, del, "delete should succeed")
	if !strings.Contains(del.Stdout, "✅ Deleted to-be-deleted") {
		t.Fatalf("unexpected delete output:\n%s", del.Stdout)
	}

	// List should no longer show it (by default)
	list := runCLI(t, bin, workDir, "list")
	requireSuccess(t, list, "list after delete should succeed")
	if strings.Contains(list.Stdout, "to-be-deleted") {
		t.Fatalf("deleted entity still appears in list:\n%s", list.Stdout)
	}
}

// TestE2E_TempStoresAreIsolated proves that using different workDirs produces
// completely separate stores (important sanity check for the harness).
func TestE2E_TempStoresAreIsolated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI E2E in short mode")
	}

	bin := buildOntologyBinary(t)

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Create different entities in each
	runCLI(t, bin, dir1, "create", "Bead", "only-in-dir1", "-p", "title=In dir1,kind=task,status=open")
	runCLI(t, bin, dir2, "create", "Bead", "only-in-dir2", "-p", "title=In dir2,kind=task,status=open")

	// Diagnostic: show what files actually exist on disk after the creates
	for name, d := range map[string]string{"dir1": dir1, "dir2": dir2} {
		graph := filepath.Join(d, "graph.jsonl")
		if b, err := os.ReadFile(graph); err == nil {
			t.Logf("DEBUG %s graph.jsonl (%d bytes): %s", name, len(b), strings.TrimSpace(string(b)))
		} else {
			t.Logf("DEBUG %s graph.jsonl does not exist: %v", name, err)
		}
	}

	get1 := runCLI(t, bin, dir1, "get", "only-in-dir1")
	requireSuccess(t, get1, "dir1 should contain its entity")

	get2 := runCLI(t, bin, dir2, "get", "only-in-dir2")
	requireSuccess(t, get2, "dir2 should contain its entity")

	// Cross-gets must not see the other store's data (the key isolation property).
	// Note: many commands currently print "❌ ..." and return without setting a non-zero
	// exit code (cobra limitation in current command implementations). We therefore
	// assert on output content rather than exit code for "not found" cases.
	missing := runCLI(t, bin, dir1, "get", "only-in-dir2")
	if strings.Contains(missing.Stdout, "only-in-dir2") && !strings.Contains(missing.Stdout, "not found") {
		t.Fatalf("cross-dir get unexpectedly saw entity from the other store (store leak?):\n%s", missing.Stdout)
	}
	// It should have emitted the not-found message (current CLI behavior)
	if !strings.Contains(missing.Stdout, "not found") {
		t.Fatalf("expected 'not found' message for cross-dir get, got:\n%s", missing.Stdout)
	}
}

// small helper so tests can sleep if we ever need to wait for file system
// (currently unused but kept for future timestamp-sensitive E2E cases).
func waitABit() { time.Sleep(5 * time.Millisecond) }
