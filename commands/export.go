package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/drover-org/drover-ontology/internal/ontology"
	"github.com/spf13/cobra"
)

var (
	exportFormat string
	exportOutput string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export active ontology to jsonld, dot, cypher, or owl",
	Long: `Extracts the active knowledge graph from the store and translates it into
standard formats for querying, visualization, and semantic interoperability.

Supported Formats:
- jsonld: JSON-LD format for semantic web tools and data sharing
- dot: Graphviz DOT format for visual rendering (e.g. dot -Tpng output.dot -o graph.png)
- cypher: A SQL/Cypher script of CREATE statements compatible with LadybugDB, Kuzu, and Neo4j
- owl: A basic XML/RDF OWL ontology format
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ont := getOntology()
		if ont == nil {
			return fmt.Errorf("ontology not initialized")
		}

		// Filter out tombstoned or invalidated entities
		var activeEntities []*ontology.Entity
		for _, e := range ont.All() {
			if !ontology.IsDeleted(e) {
				activeEntities = append(activeEntities, e)
			}
		}

		format := strings.ToLower(exportFormat)
		if format == "all" {
			var paths map[string]string
			if exportOutput == "" {
				// Create .ontology dir if not exists
				if err := os.MkdirAll(".ontology", 0755); err != nil {
					return fmt.Errorf("failed to create .ontology directory: %w", err)
				}
				paths = map[string]string{
					"jsonld": ".ontology/graph.jsonld",
					"dot":    ".ontology/graph.dot",
					"cypher": ".ontology/import.cypher",
					"owl":    ".ontology/ontology.owl",
				}
			} else {
				isDir := false
				if fi, err := os.Stat(exportOutput); err == nil && fi.IsDir() {
					isDir = true
				} else if strings.HasSuffix(exportOutput, "/") || strings.HasSuffix(exportOutput, "\\") {
					isDir = true
				}

				if isDir {
					paths = map[string]string{
						"jsonld": filepath.Join(exportOutput, "graph.jsonld"),
						"dot":    filepath.Join(exportOutput, "graph.dot"),
						"cypher": filepath.Join(exportOutput, "import.cypher"),
						"owl":    filepath.Join(exportOutput, "ontology.owl"),
					}
				} else {
					base := exportOutput
					exts := []string{".jsonld", ".dot", ".cypher", ".owl"}
					for _, ext := range exts {
						if strings.HasSuffix(strings.ToLower(base), ext) {
							base = base[:len(base)-len(ext)]
							break
						}
					}
					paths = map[string]string{
						"jsonld": base + ".jsonld",
						"dot":    base + ".dot",
						"cypher": base + ".cypher",
						"owl":    base + ".owl",
					}
				}
			}

			// Ensure directories for output files exist
			for _, p := range paths {
				dir := filepath.Dir(p)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
			}

			// Export all formats
			for _, fmtName := range []string{"jsonld", "dot", "cypher", "owl"} {
				path := paths[fmtName]
				file, err := os.Create(path)
				if err != nil {
					return fmt.Errorf("failed to create %s file %s: %w", fmtName, path, err)
				}
				var exportErr error
				switch fmtName {
				case "jsonld":
					exportErr = exportJSONLD(file, activeEntities)
				case "dot":
					exportErr = exportDOT(file, activeEntities)
				case "cypher":
					exportErr = exportCypher(file, activeEntities)
				case "owl":
					exportErr = exportOWL(file, activeEntities)
				}
				file.Close()
				if exportErr != nil {
					return fmt.Errorf("failed to export format %s to %s: %w", fmtName, path, exportErr)
				}
				fmt.Printf("Exported %s to %s\n", fmtName, path)
			}
			return nil
		}

		var writer io.Writer = os.Stdout
		if exportOutput != "" {
			file, err := os.Create(exportOutput)
			if err != nil {
				return fmt.Errorf("failed to create output file: %w", err)
			}
			defer file.Close()
			writer = file
		}

		switch format {
		case "jsonld":
			return exportJSONLD(writer, activeEntities)
		case "dot":
			return exportDOT(writer, activeEntities)
		case "cypher":
			return exportCypher(writer, activeEntities)
		case "owl":
			return exportOWL(writer, activeEntities)
		default:
			return fmt.Errorf("unsupported export format: %s", exportFormat)
		}
	},
}

func init() {
	exportCmd.Flags().StringVarP(&exportFormat, "format", "f", "jsonld", "Export format (jsonld, dot, cypher, owl, all)")
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file path or directory (default: stdout)")
}

// exportJSONLD converts active entities to standard JSON-LD
func exportJSONLD(w io.Writer, entities []*ontology.Entity) error {
	var graph []any
	for _, e := range entities {
		node := map[string]any{
			"@id":   e.ID,
			"@type": e.Type,
		}
		for k, v := range e.Properties {
			node[k] = v
		}
		// Materialize relations
		for _, r := range e.Relations {
			// Check if target is not deleted
			node[r.Type] = map[string]string{"@id": r.To}
		}
		graph = append(graph, node)
	}

	jsonld := map[string]any{
		"@context": "https://schema.org",
		"@graph":   graph,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonld)
}

// exportDOT converts active entities to Graphviz DOT format with beautiful modern colors
func exportDOT(w io.Writer, entities []*ontology.Entity) error {
	var b strings.Builder
	b.WriteString("digraph G {\n")
	b.WriteString("  node [shape=box, style=\"filled,rounded\", fillcolor=\"#EBF5FB\", fontname=\"Helvetica\", fontsize=10];\n")
	b.WriteString("  edge [fontname=\"Helvetica\", fontsize=8, color=\"#7F8C8D\"];\n\n")

	// Collect active entity IDs for relationship validation (do not draw edges to deleted nodes)
	activeIDs := make(map[string]bool, len(entities))
	for _, e := range entities {
		activeIDs[e.ID] = true
	}

	// Draw Nodes
	for _, e := range entities {
		color := getNodeColor(e.Type)
		b.WriteString(fmt.Sprintf("  %q [label=%q, fillcolor=%q];\n", e.ID, fmt.Sprintf("%s\\n(%s)", e.ID, e.Type), color))
	}
	b.WriteString("\n  // Relations\n")

	// Draw Edges
	for _, e := range entities {
		for _, r := range e.Relations {
			if activeIDs[r.To] {
				b.WriteString(fmt.Sprintf("  %q -> %q [label=%q];\n", e.ID, r.To, r.Type))
			}
		}
	}
	b.WriteString("}\n")

	_, err := io.WriteString(w, b.String())
	return err
}

func getNodeColor(typ string) string {
	switch strings.ToLower(typ) {
	case "bead", "task":
		return "#D5F5E3" // Soft Mint Green
	case "plan":
		return "#FADBD8" // Soft Salmon/Coral
	case "term":
		return "#E8F8F5" // Soft Pale Teal
	case "concept":
		return "#EAECEE" // Sleek Light Silver
	case "person":
		return "#FCF3CF" // Soft Mellow Yellow
	case "review":
		return "#E8DAEF" // Soft Pastel Purple
	default:
		return "#EBF5FB" // Soft Aerodynamic Blue
	}
}

// exportCypher converts active entities to a Cypher script for LadybugDB/Kuzu/Neo4j
func exportCypher(w io.Writer, entities []*ontology.Entity) error {
	var b strings.Builder
	b.WriteString("// Cypher import script generated by drover-ontology\n")
	b.WriteString("// Compatible with LadybugDB, KuzuDB, and Neo4j\n\n")

	// Collect active entity IDs for relation validation
	activeIDs := make(map[string]bool, len(entities))
	for _, e := range entities {
		activeIDs[e.ID] = true
	}

	// 1. Create all Nodes
	b.WriteString("// Nodes\n")
	for _, e := range entities {
		nodeVar := sanitizeVar(e.ID)
		propsBytes, _ := json.Marshal(e.Properties)
		propsStr := string(propsBytes)
		if propsStr == "null" || propsStr == "" {
			propsStr = "{}"
		}
		b.WriteString(fmt.Sprintf("CREATE (%s:%s {id: '%s', properties: %s})\n", nodeVar, e.Type, e.ID, propsStr))
	}

	// 2. Create all Relations
	b.WriteString("\n// Relationships\n")
	for _, e := range entities {
		nodeVar := sanitizeVar(e.ID)
		for _, r := range e.Relations {
			if activeIDs[r.To] {
				toVar := sanitizeVar(r.To)
				b.WriteString(fmt.Sprintf("CREATE (%s)-[:%s]->(%s)\n", nodeVar, strings.ToUpper(r.Type), toVar))
			}
		}
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func sanitizeVar(id string) string {
	res := strings.ReplaceAll(id, ":", "_")
	res = strings.ReplaceAll(res, "/", "_")
	res = strings.ReplaceAll(res, "-", "_")
	res = strings.ReplaceAll(res, " ", "_")
	res = strings.ReplaceAll(res, "@", "_")
	res = strings.ReplaceAll(res, ".", "_")
	if len(res) > 0 && res[0] >= '0' && res[0] <= '9' {
		res = "_" + res
	}
	return res
}

// exportOWL converts active entities to XML/RDF OWL Ontology format
func exportOWL(w io.Writer, entities []*ontology.Entity) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?>
<rdf:RDF xmlns="http://drover.org/ontology#"
     xml:base="http://drover.org/ontology"
     xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
     xmlns:owl="http://www.w3.org/2002/07/owl#"
     xmlns:xml="http://www.w3.org/XML/1998/namespace"
     xmlns:xsd="http://www.w3.org/2001/XMLSchema#"
     xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#">
    <owl:Ontology rdf:about="http://drover.org/ontology"/>

`)

	// 1. Declare Classes (unique Entity Types)
	b.WriteString("    <!-- Classes -->\n")
	types := make(map[string]bool)
	for _, e := range entities {
		types[e.Type] = true
	}
	for t := range types {
		b.WriteString(fmt.Sprintf("    <owl:Class rdf:about=\"#%s\"/>\n", t))
	}

	// 2. Declare Individuals
	b.WriteString("\n    <!-- Individuals -->\n")
	activeIDs := make(map[string]bool, len(entities))
	for _, e := range entities {
		activeIDs[e.ID] = true
	}

	for _, e := range entities {
		b.WriteString(fmt.Sprintf("    <owl:NamedIndividual rdf:about=\"#%s\">\n", e.ID))
		b.WriteString(fmt.Sprintf("        <rdf:type rdf:resource=\"#%s\"/>\n", e.Type))
		
		// Properties as annotations
		for k, v := range e.Properties {
			valStr := fmt.Sprintf("%v", v)
			valStr = strings.ReplaceAll(valStr, "&", "&amp;")
			valStr = strings.ReplaceAll(valStr, "<", "&lt;")
			valStr = strings.ReplaceAll(valStr, ">", "&gt;")
			b.WriteString(fmt.Sprintf("        <rdfs:comment rdf:datatype=\"http://www.w3.org/2001/XMLSchema#string\">%s: %s</rdfs:comment>\n", k, valStr))
		}

		// Relations as object assertions
		for _, r := range e.Relations {
			if activeIDs[r.To] {
				b.WriteString(fmt.Sprintf("        <owl:sameAs rdf:resource=\"#%s\"/> <!-- Relation: %s -->\n", r.To, r.Type))
			}
		}
		b.WriteString("    </owl:NamedIndividual>\n\n")
	}

	b.WriteString("</rdf:RDF>\n")

	_, err := io.WriteString(w, b.String())
	return err
}
