package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/drover-org/drover-ontology/internal/ontology"
)

func TestExportFormats(t *testing.T) {
	// Create sample active entities with relations
	entities := []*ontology.Entity{
		{
			ID:   "Term:asdlc/governance",
			Type: "Term",
			Properties: map[string]any{
				"label":      "ASDL C Governance",
				"definition": "Simple term governance framework",
				"status":     "accepted",
			},
			Relations: []ontology.Relation{
				{Type: "governed_by", To: "Person:peter-hanssens"},
			},
		},
		{
			ID:   "Person:peter-hanssens",
			Type: "Person",
			Properties: map[string]any{
				"name": "Peter Hanssens",
				"role": "Taxonomy Curator",
			},
		},
	}

	t.Run("JSON-LD", func(t *testing.T) {
		var buf bytes.Buffer
		err := exportJSONLD(&buf, entities)
		if err != nil {
			t.Fatalf("jsonld export failed: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("jsonld output is not valid JSON: %v", err)
		}

		if parsed["@context"] != "https://schema.org" {
			t.Errorf("expected context to be https://schema.org, got %v", parsed["@context"])
		}

		graph, ok := parsed["@graph"].([]any)
		if !ok || len(graph) != 2 {
			t.Fatalf("expected @graph to contain 2 elements, got %v", parsed["@graph"])
		}

		first := graph[0].(map[string]any)
		if first["@id"] != "Term:asdlc/governance" || first["@type"] != "Term" {
			t.Errorf("unexpected first graph node: %v", first)
		}
	})

	t.Run("DOT", func(t *testing.T) {
		var buf bytes.Buffer
		err := exportDOT(&buf, entities)
		if err != nil {
			t.Fatalf("dot export failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "digraph G {") {
			t.Errorf("expected DOT graph declaration, got %s", output)
		}
		if !strings.Contains(output, `"Term:asdlc/governance"`) || !strings.Contains(output, `fillcolor="#E8F8F5"`) {
			t.Errorf("expected Term node with soft teal fillcolor, got %s", output)
		}
		if !strings.Contains(output, `"Term:asdlc/governance" -> "Person:peter-hanssens" [label="governed_by"];`) {
			t.Errorf("expected governed_by edge, got %s", output)
		}
	})

	t.Run("Cypher", func(t *testing.T) {
		var buf bytes.Buffer
		err := exportCypher(&buf, entities)
		if err != nil {
			t.Fatalf("cypher export failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "CREATE (Term_asdlc_governance:Term") {
			t.Errorf("expected CREATE statement for Term, got %s", output)
		}
		if !strings.Contains(output, "CREATE (Person_peter_hanssens:Person") {
			t.Errorf("expected CREATE statement for Person, got %s", output)
		}
		if !strings.Contains(output, "CREATE (Term_asdlc_governance)-[:GOVERNED_BY]->(Person_peter_hanssens)") {
			t.Errorf("expected CREATE relationship statement, got %s", output)
		}
	})

	t.Run("OWL", func(t *testing.T) {
		var buf bytes.Buffer
		err := exportOWL(&buf, entities)
		if err != nil {
			t.Fatalf("owl export failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "<owl:Ontology rdf:about=\"http://drover.org/ontology\"/>") {
			t.Errorf("expected OWL ontology declaration, got %s", output)
		}
		if !strings.Contains(output, "<owl:Class rdf:about=\"#Term\"/>") {
			t.Errorf("expected Class Term declaration, got %s", output)
		}
		if !strings.Contains(output, "<owl:NamedIndividual rdf:about=\"#Term:asdlc/governance\">") {
			t.Errorf("expected NamedIndividual declaration, got %s", output)
		}
		if !strings.Contains(output, "<owl:sameAs rdf:resource=\"#Person:peter-hanssens\"/> <!-- Relation: governed_by -->") {
			t.Errorf("expected sameAs relation link, got %s", output)
		}
	})
}

func TestExportAll(t *testing.T) {
	// Create a temp directory for tests
	tmpDir := t.TempDir()

	// Write basic schema file
	schemaContent := `
types:
  Term:
    required: [label]
  Person:
    required: [name]
`
	schemaPath := filepath.Join(tmpDir, "schema.yaml")
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0644); err != nil {
		t.Fatalf("failed to write schema.yaml: %v", err)
	}

	// Initialize ontologyInstance
	cfg := ontology.Config{
		GraphPath:  filepath.Join(tmpDir, "graph.jsonl"),
		DBPath:     filepath.Join(tmpDir, "graph.db"),
		SchemaPath: schemaPath,
		UseSQLite:  true,
	}
	ont, err := ontology.New(cfg)
	if err != nil {
		t.Fatalf("failed to initialize ontology: %v", err)
	}

	// Create some dummy entities
	err = ont.CreateEntity("Term", "Term:asdlc/governance", map[string]any{"label": "ASDL C"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}
	err = ont.CreateEntity("Person", "Person:peter-hanssens", map[string]any{"name": "Peter"})
	if err != nil {
		t.Fatalf("failed to create entity: %v", err)
	}
	err = ont.Relate("Term:asdlc/governance", "governed_by", "Person:peter-hanssens", nil)
	if err != nil {
		t.Fatalf("failed to relate entities: %v", err)
	}

	// Backup and set the package global ontologyInstance
	oldOnt := ontologyInstance
	ontologyInstance = ont
	defer func() {
		ontologyInstance = oldOnt
	}()

	// Case 1: exportOutput is empty (defaults to .ontology/)
	oldFormat := exportFormat
	oldOutput := exportOutput
	defer func() {
		exportFormat = oldFormat
		exportOutput = oldOutput
	}()

	t.Run("Format all with directory output", func(t *testing.T) {
		outDir := filepath.Join(tmpDir, "exports")
		if err := os.MkdirAll(outDir, 0755); err != nil {
			t.Fatalf("failed to create outDir: %v", err)
		}
		exportFormat = "all"
		exportOutput = outDir

		// Call RunE manually
		err := exportCmd.RunE(exportCmd, nil)
		if err != nil {
			t.Fatalf("export all failed: %v", err)
		}

		// Verify files exist
		expectedFiles := []string{
			filepath.Join(outDir, "graph.jsonld"),
			filepath.Join(outDir, "graph.dot"),
			filepath.Join(outDir, "import.cypher"),
			filepath.Join(outDir, "ontology.owl"),
		}
		for _, f := range expectedFiles {
			if _, err := os.Stat(f); os.IsNotExist(err) {
				t.Errorf("expected file %s to be created, but it does not exist", f)
			}
		}
	})

	t.Run("Format all with base prefix output", func(t *testing.T) {
		outPrefix := filepath.Join(tmpDir, "prefix-export")
		exportFormat = "all"
		exportOutput = outPrefix

		// Call RunE manually
		err := exportCmd.RunE(exportCmd, nil)
		if err != nil {
			t.Fatalf("export all failed: %v", err)
		}

		// Verify files exist
		expectedFiles := []string{
			outPrefix + ".jsonld",
			outPrefix + ".dot",
			outPrefix + ".cypher",
			outPrefix + ".owl",
		}
		for _, f := range expectedFiles {
			if _, err := os.Stat(f); os.IsNotExist(err) {
				t.Errorf("expected file %s to be created, but it does not exist", f)
			}
		}
	})
}

