package ontology

import (
	"time"
)

// Operation (formerly Mutation) is the canonical append-only event format.
// This is the single source of truth for all changes to the ontology.
type Operation struct {
	Op         string                 `json:"op"`
	Entity     *Entity                `json:"entity,omitempty"`
	ID         string                 `json:"id,omitempty"`
	Set        map[string]any         `json:"set,omitempty"`
	From       string                 `json:"from,omitempty"`
	To         string                 `json:"to,omitempty"`
	Rel        string                 `json:"rel,omitempty"`
	Properties map[string]any         `json:"properties,omitempty"`
	Reason     string                 `json:"reason,omitempty"`
	Ts         time.Time              `json:"ts,omitempty"`
	Source     string                 `json:"source,omitempty"`
	ValidUntil string                 `json:"valid_until,omitempty"`
}

// Entity is the canonical representation of a node in the knowledge graph.
// It includes temporal validity and materialized relations for convenience.
type Entity struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Properties map[string]any         `json:"properties"`
	Relations  []Relation             `json:"relations,omitempty"`
	Created    time.Time              `json:"created"`
	Updated    time.Time              `json:"updated"`
	ValidFrom  time.Time              `json:"valid_from"`
	ValidUntil *time.Time             `json:"valid_until,omitempty"`
	Source     string                 `json:"source,omitempty"`
}

// Relation represents a directed edge from one Entity to another.
type Relation struct {
	Type       string         `json:"type"`
	To         string         `json:"to"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Schema holds the constraint definitions for Entities and Relations
// (compatible with drover-schema.yaml and core concepts like Bead, Plan, depends_on, etc.).
type Schema struct {
	Types     map[string]TypeDef     `yaml:"types" json:"types"`
	Relations map[string]RelationDef `yaml:"relations" json:"relations"`
	Policies  []string               `yaml:"policies" json:"policies"`
}

type TypeDef struct {
	Required   []string           `yaml:"required"`
	Optional   []string           `yaml:"optional"`
	Properties map[string]any     `yaml:"properties,omitempty"`
}

type RelationDef struct {
	FromTypes   []string `yaml:"from_types"`
	ToTypes     []string `yaml:"to_types"`
	Cardinality string   `yaml:"cardinality"`
	Acyclic     bool     `yaml:"acyclic"`
	Description string   `yaml:"description"`
}

// ValidationError represents a failure of schema or policy rules.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

// NewEntity is a helper for creating minimal valid entities.
func NewEntity(id, typ string, props map[string]any) *Entity {
	now := time.Now().UTC()
	if props == nil {
		props = map[string]any{}
	}
	return &Entity{
		ID:         id,
		Type:       typ,
		Properties: props,
		Relations:  []Relation{},
		Created:    now,
		Updated:    now,
		ValidFrom:  now,
	}
}

// IsDeleted reports whether the entity is considered deleted for default
// user-facing query surfaces (list, get, search, etc.).
//
// An entity is deleted if:
// - its "status" property is "tombstoned", or
// - it has a ValidUntil in the past.
//
// This implements the visibility semantics for the `delete` command
// (which uses tombstone + invalidate).
func IsDeleted(e *Entity) bool {
	if e == nil {
		return true
	}
	if s, ok := e.Properties["status"].(string); ok && s == "tombstoned" {
		return true
	}
	if e.ValidUntil != nil && e.ValidUntil.Before(time.Now().UTC()) {
		return true
	}
	return false
}
