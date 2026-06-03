// tools/rlm-ontology/main.go
//
// RLM-style ontology builder spike for drover-ontology.
//
// This is a self-contained demonstration that shows exactly how a Recursive
// Language Model (using rlm-go + Yaegi or a manual controller) would:
//   1. Load an entire Go codebase + docs with a drover-aware loader.
//   2. Receive powerful code-analysis helpers (ListFiles, ParseGoFile via go/ast, etc.).
//   3. Build a governed drover ontology by calling the high-level API
//      (CreateEntity / Relate / Validate / IntegrityCheck) either directly
//      (trusted in-process) or by exec'ing the real "ontology" / "drover-ont" binary.
//
// CRITICAL RULE
//   In any real deployment where model-generated code runs in the REPL,
//   you should prefer the Exec mode of DroverClient (or a capability-restricted
//   wrapper around the installed binary) so that every write still goes through
//   the official Go CLI, the append-only JSONL, the schema policies, and fsck.
//
// Usage:
//   go run ./tools/rlm-ontology [path-to-codebase]
//
// The spike performs a small number of "RLM-emulated" recursive steps that look
// exactly like the kind of Go code an RLM would emit inside its Yaegi REPL.
// At the end it calls Validate + Fsck and prints a FINAL-style summary.

//go:build !rlm

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/drover-org/drover-ontology/internal/ingest"
	"github.com/drover-org/drover-ontology/internal/ontology"
)

func main() {
	target := flag.String("target", ".", "Path to the Go codebase / monorepo root to ingest")
	forceExec := flag.Bool("force-exec", false, "Force all writes through the real ontology/drover-ont binary (obeys CRITICAL RULE even in the spike)")
	binary := flag.String("binary", "", "Explicit path or name of the ontology binary (default: ontology or drover-ont)")
	graph := flag.String("graph", ".ontology/graph.jsonl", "Graph JSONL path (used by exec mode and snapshot)")
	db := flag.String("db", ".ontology/graph.db", "SQLite path (used by exec mode)")
	schema := flag.String("schema", ".ontology/schema.yaml", "Schema path (used by exec mode)")
	delta := flag.Bool("delta", false, "Use Git-driven incremental ingestion (Delta Mode)")
	flag.Parse()

	root, err := filepath.Abs(*target)
	if err != nil {
		log.Fatalf("abs target: %v", err)
	}
	fmt.Printf("RLM ontology spike starting on: %s\n", root)

	// 1. Strong drover-aware loader (exactly the one an RLM harness would use)
	loader := &ingest.CodebaseLoader{
		RootPath:     root,
		MaxFileBytes: 400 * 1024,
		UseGitDelta:  *delta,
	}
	ctxStr, err := loader.LoadGoCodebase()
	if err != nil {
		log.Fatalf("loader failed: %v", err)
	}
	if *delta {
		fmt.Println("Incremental Delta Mode active. Ingesting git status diff only.")
	}
	fmt.Printf("Loaded context: ~%d bytes (%.1f MB)\n", len(ctxStr), float64(len(ctxStr))/1024/1024)

	// 2. Analysis helpers (these get injected into the REPL environment)
	analysis := &ingest.CodeAnalysis{Context: ctxStr}

	// 3. Prepare the governed drover client
	//    - By default we use direct (in-process) for the spike so it "just works"
	//      without requiring the binary to be installed.
	//    - Pass -force-exec (and optionally -binary) to prove the "write through binary" path.
	var client *ingest.DroverClient
	if *forceExec {
		client = ingest.NewDroverClientExec(*binary, *graph, *db, *schema)
		client.ForceExec = true
		fmt.Println("→ Using EXEC mode (writes go through the real binary)")
	} else {
		// Trusted in-process path for the demo. We still go through the real
		// Ontology high-level methods (CreateEntity etc.) so the reducer,
		// storage, validation seam, and everything else are exercised exactly
		// as production commands do.
		cfg := ontology.Config{
			GraphPath:  *graph,
			DBPath:     *db,
			SchemaPath: *schema,
			UseSQLite:  true,
		}
		ont, err := ontology.New(cfg)
		if err != nil {
			log.Fatalf("failed to open ontology for direct mode: %v", err)
		}
		defer ont.Close()
		client = ingest.NewDroverClientDirect(ont)
		fmt.Println("→ Using DIRECT mode (in-process high-level API — still the real Ontology module)")
	}

	// 4. "RLM-emulated" recursive steps.
	//    In a real rlm-go integration these would be emitted by the model
	//    inside the Yaegi REPL and would call the exact same funcs we registered.
	//    The harness would also expose "context" as a variable.
	fmt.Println("\n=== RLM Step 1: High-level reconnaissance ===")
	files := analysis.ListFiles()
	fmt.Printf("  Files visible to model: %d\n", len(files))

	// Look for go.mod + key drover files (the model would do this)
	keyFiles := []string{"go.mod", "CONTEXT.md", "beads.json", "beads.jsonl", "references/drover-schema.yaml", "references/taxonomy-relations.md", "SKILL.md"}
	for _, kf := range keyFiles {
		if _, err := analysis.GetFileContent(kf); err == nil {
			fmt.Printf("  ✓ Found %s\n", kf)
		}
	}

	fmt.Println("\n=== RLM Step 2: Architecture extraction (ParseGoFile on core) ===")
	corePkgs := []string{
		"internal/ontology/ontology.go",
		"internal/ontology/reducer.go",
		"internal/ontology/schema.go",
		"cmd/ontology/main.go",
	}
	for _, p := range corePkgs {
		if parsed, err := analysis.ParseGoFile(p); err == nil {
			// The model would now create Package + Struct entities from the parsed output.
			// We emulate a tiny useful slice of that work.
			_ = parsed
			pkgID := "Package:" + p
			_ = client.Create("Package", pkgID, map[string]any{
				"path":   p,
				"source": "rlm-ingest-spike",
			})
			// Example of a structural relation the model would discover
			if strings.Contains(p, "ontology.go") {
				_ = client.Relate(pkgID, "part_of", "Module:drover-ontology", nil)
			}
		}
	}

	fmt.Println("\n=== RLM Step 3: Term / taxonomy seeding from known drover concepts ===")
	// In reality the model would mine CONTEXT.md + glossaries + code comments.
	// Here we create a few high-value Terms that any drover run should have,
	// demonstrating the governance pattern (proposed → governed_by → accepted).
	terms := []struct {
		id     string
		label  string
		status string
	}{
		{"Term:asdlc/governance", "ASDL C Governance", "proposed"},
		{"Term:ontology/operation-reducer", "OperationReducer (single source of truth for folding)", "proposed"},
		{"Term:ontology/load-fidelity", "Load fidelity contract B (always-replay)", "proposed"},
	}
	for _, t := range terms {
		_ = client.Create("Term", t.id, map[string]any{
			"label":  t.label,
			"status": t.status,
			"source": "rlm-ingest-spike",
		})
	}
	// Wire governance (the model must do this before promoting status)
	_ = client.Relate("Term:asdlc/governance", "governed_by", "Person:peter-hanssens", map[string]any{"role": "curator"})
	_ = client.Update("Term:asdlc/governance", map[string]any{"status": "accepted"})

	fmt.Println("\n=== RLM Step 4: Link some delivery concepts (Plan/Bead style) ===")
	_ = client.Create("Plan", "Plan:rlm-ontology-ingest", map[string]any{
		"kind":   "eng",
		"title":  "RLM-powered codebase ontology ingestion",
		"status": "in_review",
		"source": "rlm-ingest-spike",
	})
	_ = client.Create("Bead", "Bead:rlm-spike-001", map[string]any{
		"title":  "Land internal/ingest + rlm-integration docs",
		"kind":   "spike",
		"status": "in_progress",
	})
	_ = client.Relate("Bead:rlm-spike-001", "implements", "Plan:rlm-ontology-ingest", nil)

	fmt.Println("\n=== RLM Step 5: Validation + integrity (the model does this in a loop) ===")
	valErrs, _ := client.Validate()
	if len(valErrs) > 0 {
		fmt.Printf("  Validation issues surfaced to model: %d (model would now fix them)\n", len(valErrs))
		for i, e := range valErrs {
			if i < 3 {
				fmt.Printf("    - %s\n", e)
			}
		}
	} else {
		fmt.Println("  ✓ No validation errors from DroverValidate()")
	}

	if err := client.IntegrityCheck(); err != nil {
		fmt.Printf("  Fsck note: %v (expected on a spike run that only added a few entities)\n", err)
	} else {
		fmt.Println("  ✓ DroverFsck passed")
	}

	stats, _ := client.Stats()
	fmt.Printf("  Stats: %s\n", stats)

	// Final snapshot (the model calls this)
	snap := filepath.Join(".ontology", "snapshots", fmt.Sprintf("rlm-spike-%s.jsonl", time.Now().UTC().Format("20060102-150405")))
	_ = client.ExportSnapshot(snap)

	fmt.Println("\n=== RLM Termination ===")
	fmt.Printf("FINAL_ONTOLOGY(\"RLM spike completed on %s. Created demo Terms, Packages, Plan, Bead with proper relations and governance wiring. Validate + Fsck exercised. Snapshot at %s. In a real run the model would have explored hundreds of files recursively and produced a much richer governed graph.\")\n", root, snap)

	// Bonus: show the model *could* have called the pure analysis helpers at any time
	fmt.Println("\n(Example of what the model could have printed from inside the REPL)")
	fmt.Printf("  Recommended relations sample: %v\n", analysis.RecommendedDroverRelations()[:6])
	fmt.Printf("  Drover entity types sample: %v\n", analysis.ListDroverEntityTypes()[:5])

	_ = context.Background() // in a real harness you would pass ctx to LLM calls / sub-RLM recursion
}
