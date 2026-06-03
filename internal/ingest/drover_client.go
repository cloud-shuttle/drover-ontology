package ingest

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/drover-org/drover-ontology/internal/ontology"
)

// DroverClient is the bridge that an RLM (or any automated ingester) uses to
// write into the governed drover-ontology store.
//
// CRITICAL RULE (enforced by design):
//   All real production writes must ultimately go through the installed Go binary
//   ("ontology" or the "drover-ont" monorepo wrapper). This preserves the single
//   source of truth, the append-only JSONL, the schema policies (governed_by,
//   approved-Plan-requires-Review, Term governance, acyclicity), temporal validity,
//   fsck, and the PBT regression guards.
//
// The client supports two modes:
//   1. Direct (in-process): holds an *ontology.Ontology and calls the high-level
//      CreateEntity/Relate/Update/SoftDelete/Validate/IntegrityCheck methods.
//      Excellent for trusted demos, tests, and when the RLM controller already
//      owns the Ontology instance.
//   2. Exec (binary): shells out to "ontology" or "drover-ont" (or a caller-
//      supplied absolute path). This is the path that untrusted Yaegi-generated
//      code from an RLM should be routed through.
//
// In a real rlm-go + Yaegi integration, you register only the thin wrapper funcs
// (DroverCreate, DroverRelate, ...) into the interpreter environment. The model
// writes Go code that calls those funcs; the funcs themselves decide direct vs exec.
type DroverClient struct {
	// Direct mode
	ont *ontology.Ontology

	// Exec mode
	binaryPath string // "ontology", "drover-ont", or absolute path to the binary
	graphPath  string // passed as --graph
	dbPath     string // --db
	schemaPath string // --schema

	// When true, prefer exec even if ont != nil (useful for "prove we went through the binary" demos)
	ForceExec bool
}

// NewDroverClientDirect returns a client that writes directly via the high-level
// Ontology API (CreateEntity, Relate, etc.). Use this inside the same process
// as a trusted controller (the common case for a demo spike or a controlled
// RLM harness that already did ontology.New(cfg)).
func NewDroverClientDirect(ont *ontology.Ontology) *DroverClient {
	return &DroverClient{ont: ont}
}

// NewDroverClientExec returns a client that shells out to the real binary for
// every write. This is the mode that best obeys the "all agents write through
// the ontology binary" rule when the REPL is executing model-generated code.
func NewDroverClientExec(binary, graph, db, schema string) *DroverClient {
	if binary == "" {
		binary = "ontology"
		// Inside drover-org monorepo the wrapper is usually "drover-ont"
		if _, err := exec.LookPath("drover-ont"); err == nil {
			binary = "drover-ont"
		}
	}
	return &DroverClient{
		binaryPath: binary,
		graphPath:  graph,
		dbPath:     db,
		schemaPath: schema,
	}
}

// SetDirect allows runtime switching (mostly for tests).
func (c *DroverClient) SetDirect(ont *ontology.Ontology) { c.ont = ont }

// useExec reports whether this call should shell out.
func (c *DroverClient) useExec() bool {
	if c.ForceExec {
		return true
	}
	return c.ont == nil || c.binaryPath != ""
}

// runBinary executes the configured binary with the given args and --graph/--db/--schema.
// It returns combined stdout+stderr and an error if exit status != 0.
func (c *DroverClient) runBinary(args ...string) (string, error) {
	bin := c.binaryPath
	if bin == "" {
		bin = "ontology"
	}

	full := []string{}
	if c.graphPath != "" {
		full = append(full, "--graph", c.graphPath)
	}
	if c.dbPath != "" {
		full = append(full, "--db", c.dbPath)
	}
	if c.schemaPath != "" {
		full = append(full, "--schema", c.schemaPath)
	}
	full = append(full, args...)

	cmd := exec.Command(bin, full...)
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w\noutput:\n%s", bin, strings.Join(full, " "), err, string(out))
	}
	return string(out), nil
}

// Create creates a new entity via the high-level API or the binary.
// Props may be nil. The id should be stable and human-meaningful when possible
// (e.g. "Term:asdlc/governance" or "Package:internal/ingest").
func (c *DroverClient) Create(typ, id string, props map[string]any) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if props == nil {
		props = map[string]any{}
	}

	if !c.useExec() {
		return c.ont.CreateEntity(typ, id, props)
	}

	// Exec path: build -p flags
	args := []string{"create", typ, id}
	for k, v := range props {
		// Best-effort stringification; the binary accepts key=value for --prop
		args = append(args, "-p", fmt.Sprintf("%s=%v", k, v))
	}
	out, err := c.runBinary(args...)
	if err != nil {
		return err
	}
	// The real CLI prints "✅ Created ..."; we just surface it for the REPL
	fmt.Print(out)
	return nil
}

// Relate creates a directed relation (with optional props) via high-level or binary.
func (c *DroverClient) Relate(from, rel, to string, props map[string]any) error {
	if from == "" || rel == "" || to == "" {
		return fmt.Errorf("from, rel, and to are required")
	}
	if props == nil {
		props = map[string]any{}
	}

	if !c.useExec() {
		return c.ont.Relate(from, rel, to, props)
	}

	args := []string{"relate", from, rel, to}
	for k, v := range props {
		args = append(args, "-p", fmt.Sprintf("%s=%v", k, v))
	}
	out, err := c.runBinary(args...)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// Update mutates properties on an existing entity.
func (c *DroverClient) Update(id string, set map[string]any) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if set == nil {
		set = map[string]any{}
	}

	if !c.useExec() {
		return c.ont.Update(id, set)
	}

	args := []string{"update", id}
	for k, v := range set {
		args = append(args, "-p", fmt.Sprintf("%s=%v", k, v))
	}
	out, err := c.runBinary(args...)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// SoftDelete performs the governed two-step tombstone + invalidate.
func (c *DroverClient) SoftDelete(id, reason string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}

	if !c.useExec() {
		return c.ont.SoftDelete(id, reason)
	}

	args := []string{"delete", id, "--reason", reason}
	out, err := c.runBinary(args...)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}

// Validate runs the full schema + policy validation (governed_by, Plan approval,
// acyclicity, etc.) and returns the errors (or nil). In exec mode it surfaces
// the output of `ontology validate` (the CLI surfaces violations on stdout/stderr).
func (c *DroverClient) Validate() ([]string, error) {
	if !c.useExec() {
		errs := c.ont.Validate()
		out := make([]string, len(errs))
		for i, e := range errs {
			out[i] = e.Error()
		}
		return out, nil
	}

	out, err := c.runBinary("validate")
	if err != nil {
		// The CLI returns non-zero on violations; we still want the messages
		return strings.Split(strings.TrimSpace(out), "\n"), nil
	}
	return nil, nil
}

// IntegrityCheck runs fsck-style replay + policy validation on the authoritative log.
func (c *DroverClient) IntegrityCheck() error {
	if !c.useExec() {
		return c.ont.IntegrityCheck()
	}
	out, err := c.runBinary("fsck")
	if err != nil {
		return fmt.Errorf("fsck failed:\n%s", out)
	}
	fmt.Print(out)
	return nil
}

// ExportSnapshot copies the current graph.jsonl to a caller-chosen path (useful
// after a big RLM run so you have a point-in-time governed artifact).
func (c *DroverClient) ExportSnapshot(destPath string) error {
	src := c.graphPath
	if src == "" {
		src = ".ontology/graph.jsonl"
	}
	if c.useExec() {
		// Ask the binary to do a best-effort copy via a hypothetical future
		// "snapshot" subcommand, or just shell cp here (the log is the truth).
		// For now we do the safe thing: copy the file ourselves after we know
		// the binary wrote it.
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source log %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("📸 Exported snapshot: %s (%d bytes)\n", destPath, len(data))
	return nil
}

// Stats returns a tiny summary (entity count, relation count) for the RLM to
// print in its FINAL step. In direct mode it uses the in-memory view; in exec
// it falls back to a cheap list + wc or similar.
func (c *DroverClient) Stats() (string, error) {
	if !c.useExec() && c.ont != nil {
		ents := c.ont.All()
		relCount := 0
		for _, e := range ents {
			relCount += len(e.Relations)
		}
		return fmt.Sprintf("entities=%d relations=%d (direct)", len(ents), relCount), nil
	}

	// Exec mode: best effort via list (the CLI already pretty-prints)
	out, _ := c.runBinary("list", "--limit", "9999")
	lines := strings.Split(out, "\n")
	return fmt.Sprintf("list output lines=%d (via binary)", len(lines)), nil
}

// AsMap returns a small map of the callable symbols that should be injected
// into a Yaegi (or other Go interpreter) REPL environment for an RLM.
//
// Typical usage with rlm-go style injection:
//
//   env := map[string]any{
//       "context":        fullContextString,
//       "ListFiles":      analysis.ListFiles,
//       "GetFileContent": analysis.GetFileContent,
//       ...
//       "DroverCreate":   client.Create,
//       "DroverRelate":   client.Relate,
//       ...
//   }
//   // then pass env to your rlm.CompleteWithEnv or equivalent
func (c *DroverClient) AsMap(analysis *CodeAnalysis) map[string]any {
	m := map[string]any{
		"DroverCreate":   c.Create,
		"DroverRelate":   c.Relate,
		"DroverUpdate":   c.Update,
		"DroverDelete":   c.SoftDelete,
		"DroverValidate": c.Validate,
		"DroverFsck":     c.IntegrityCheck,
		"DroverStats":    c.Stats,
		"DroverSnapshot": c.ExportSnapshot,
	}
	if analysis != nil {
		m["ListFiles"] = analysis.ListFiles
		m["GetFileContent"] = analysis.GetFileContent
		m["FindReferences"] = analysis.FindReferences
		m["ParseGoFile"] = analysis.ParseGoFile
		m["LoadBeadsJSON"] = analysis.LoadBeadsJSON
		m["RecommendedDroverRelations"] = analysis.RecommendedDroverRelations
		m["ListDroverEntityTypes"] = analysis.ListDroverEntityTypes
	}
	return m
}

// Helper for quick JSON round-tripping inside REPL code the model writes.
func ToJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
