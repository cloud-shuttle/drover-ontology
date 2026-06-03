package ontology

import (
	"encoding/json"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadSchema reads a drover-schema.yaml (or .json) file. Falls back to embedded defaults.
func LoadSchema(path string) (*Schema, error) {
	if path == "" {
		return DefaultSchema(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Write default schema to path so the user has it, and return DefaultSchema
			s := DefaultSchema()
			if yamlData, yerr := yaml.Marshal(s); yerr == nil {
				_ = os.WriteFile(path, yamlData, 0644)
			}
			return s, nil
		}
		return nil, err
	}

	var s Schema
	if err := yaml.Unmarshal(data, &s); err != nil {
		// Try JSON as fallback
		if jerr := json.Unmarshal(data, &s); jerr != nil {
			return nil, err
		}
	}
	if s.Types == nil {
		s.Types = map[string]TypeDef{}
	}
	if s.Relations == nil {
		s.Relations = map[string]RelationDef{}
	}
	return &s, nil
}

// DefaultSchema is the minimal viable schema (subset of drover + core).
func DefaultSchema() *Schema {
	return &Schema{
		Types: map[string]TypeDef{
			"Bead": {
				Required: []string{"title", "kind", "status"},
			},
			"Plan": {
				Required: []string{"kind", "title", "status"},
			},
			"Review": {
				Required: []string{"kind", "decision"},
			},
			"Person":   {Required: []string{"name"}},
			"Project":  {Required: []string{"name"}},
			"Task":     {Required: []string{"title", "status"}},
			"Ticket":   {Required: []string{"key", "title", "status"}},
			"Term":     {Required: []string{"label", "definition"}},
			"Document": {Required: []string{"title"}},
		},
		Relations: map[string]RelationDef{
			"implements": {FromTypes: []string{"Bead", "Ticket"}, ToTypes: []string{"Plan", "Goal"}},
			"depends_on": {FromTypes: []string{"Bead", "Task", "Plan"}, ToTypes: []string{"Bead", "Task", "Plan"}, Acyclic: true},
			"blocks":     {FromTypes: []string{"Bead", "Task"}, ToTypes: []string{"Bead", "Task"}, Acyclic: true},
			"owns":       {FromTypes: []string{"Person"}, ToTypes: []string{"Project", "Plan", "Bead"}},
			"has_owner":  {FromTypes: []string{"Project", "Plan", "Bead"}, ToTypes: []string{"Person"}},
			"reviewed_by": {FromTypes: []string{"Plan"}, ToTypes: []string{"Review"}},
			"approves":   {FromTypes: []string{"Review"}, ToTypes: []string{"Plan"}},
			// Governance + classification (used by Term acceptance policy)
			"governed_by": {FromTypes: []string{"Term"}, ToTypes: []string{"Person", "Review", "Policy"}},
			"tagged_with": {FromTypes: []string{"Term", "Bead", "Plan", "Review", "Person"}, ToTypes: []string{"Term"}},
		},
	}
}
