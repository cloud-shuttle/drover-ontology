package commands

import (
	"fmt"
	"os"

	"github.com/drover-org/drover-ontology/internal/ontology"
	"github.com/spf13/cobra"
)

var (
	graphPath  string
	dbPath     string
	schemaPath string
)

// ontologyInstance holds the configured, deepened Ontology for the process.
// This replaces the previous global/hardcoded approach.
var ontologyInstance *ontology.Ontology

var RootCmd = &cobra.Command{
	Use:   "ontology",
	Short: "Drover Ontology - Typed knowledge graph with temporal validity, embeddings, and hybrid search",
	Long: `A powerful ontology CLI for structured agent memory.

Features:
- Temporal entity validity (valid_from / valid_until)
- Custom schema enforcement
- SQLite primary backend with JSONL audit log
- Automatic Ollama embeddings + hybrid vector/graph search
- Full embed model management`,
	SilenceUsage: true,
}

func init() {
	RootCmd.PersistentFlags().StringVar(&graphPath, "graph", ".ontology/graph.jsonl", "Path to JSONL audit log")
	RootCmd.PersistentFlags().StringVar(&dbPath, "db", ".ontology/graph.db", "Path to SQLite database")
	RootCmd.PersistentFlags().StringVar(&schemaPath, "schema", ".ontology/schema.yaml", "Path to schema definition")

	// Initialize the configured Ontology instance AFTER flags are parsed but BEFORE any subcommand Run.
	// This ensures --graph/--db/--schema are respected (previous timing in Execute() only saw defaults).
	RootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cfg := ontology.Config{
			GraphPath:  graphPath,
			DBPath:     dbPath,
			SchemaPath: schemaPath,
			EmbedModel: "nomic-embed-text",
			UseSQLite:  true,
		}
		var err error
		ontologyInstance, err = ontology.New(cfg)
		if err != nil {
			// Fall back gracefully so the tool remains usable even if DB init has issues
			fmt.Fprintf(os.Stderr, "Warning: failed to open configured ontology: %v\n", err)
			ontologyInstance, err = ontology.New(ontology.DefaultConfig())
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: fallback to default config also failed: %v\n", err)
			}
		}
		return nil
	}

	// Add all subcommands
	RootCmd.AddCommand(createCmd)
	RootCmd.AddCommand(getCmd)
	RootCmd.AddCommand(listCmd)
	RootCmd.AddCommand(updateCmd)
	RootCmd.AddCommand(deleteCmd)
	RootCmd.AddCommand(relateCmd)
	RootCmd.AddCommand(queryCmd)
	RootCmd.AddCommand(searchCmd)
	RootCmd.AddCommand(invalidateCmd)
	RootCmd.AddCommand(historyCmd)
	RootCmd.AddCommand(validateCmd)
	RootCmd.AddCommand(migrateCmd)
	RootCmd.AddCommand(embedCmd)
	RootCmd.AddCommand(fsckCmd)
	RootCmd.AddCommand(exportCmd)
	RootCmd.AddCommand(visualizeCmd)
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// GetOntology returns the configured deepened ontology instance for use by commands.
// This replaces scattered hardcoded paths and globals.
func GetOntology() *ontology.Ontology {
	return ontologyInstance
}

// getOntology is a small helper to collapse the repeated "GetOntology() + nil check + print" dance
// across commands (part of collapsing boilerplate per Candidate 04). Returns nil (after printing)
// if not initialized. Callers should `if ont == nil { return }`.
func getOntology() *ontology.Ontology {
	ont := GetOntology()
	if ont == nil {
		fmt.Println("❌ Ontology not initialized")
	}
	return ont
}

// propsFromStringMap collapses the repeated map[string]string -> map[string]any conversion
// done for -p/--prop flags (cobra gives string map, ontology uses any).
func propsFromStringMap(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	res := make(map[string]any, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}
