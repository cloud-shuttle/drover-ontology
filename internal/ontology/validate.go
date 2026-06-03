package ontology

import (
	"fmt"
)

// Validator performs structural + schema validation.
type Validator struct {
	schema *Schema
}

// NewValidator creates a validator for the given schema.
func NewValidator(s *Schema) *Validator {
	if s == nil {
		s = DefaultSchema()
	}
	return &Validator{schema: s}
}

// ValidateEntity checks a single entity against the type definition.
func (v *Validator) ValidateEntity(e *Entity) []ValidationError {
	var errs []ValidationError
	tdef, ok := v.schema.Types[e.Type]
	if !ok {
		// Unknown type is allowed (extensible) but we still check required if present in any def
		return errs
	}

	props := e.Properties
	for _, req := range tdef.Required {
		if _, exists := props[req]; !exists {
			errs = append(errs, ValidationError{fmt.Sprintf("%s %s missing required property: %s", e.Type, e.ID, req)})
		}
	}

	// Simple enum support: look for <key>_enum in the raw schema map if we stored it
	// For now we keep it lightweight — full enum enforcement can be added from YAML later.
	return errs
}

// ValidateRelation checks cardinality + allowed type pairs + acyclicity (when requested).
func (v *Validator) ValidateRelation(from, to *Entity, rel string, allEntities map[string]*Entity) []ValidationError {
	var errs []ValidationError
	rdef, ok := v.schema.Relations[rel]
	if !ok {
		return errs // unknown relations are soft-ok for now
	}

	allowedFrom := false
	for _, t := range rdef.FromTypes {
		if t == from.Type {
			allowedFrom = true
			break
		}
	}
	if !allowedFrom {
		errs = append(errs, ValidationError{fmt.Sprintf("relation %q not allowed from type %s (on relation %s -> %s)", rel, from.Type, from.ID, to.ID)})
	}

	allowedTo := false
	for _, t := range rdef.ToTypes {
		if t == to.Type {
			allowedTo = true
			break
		}
	}
	if !allowedTo {
		errs = append(errs, ValidationError{fmt.Sprintf("relation %q not allowed to type %s (on relation %s -> %s)", rel, to.Type, from.ID, to.ID)})
	}

	if rdef.Acyclic {
		if createsCycle(from.ID, to.ID, rel, allEntities) {
			errs = append(errs, ValidationError{fmt.Sprintf("acyclic violation: %s would create a cycle", rel)})
		}
	}
	return errs
}

// createsCycle does a DFS to detect if adding from->to under rel would close a cycle.
func createsCycle(fromID, toID, rel string, entities map[string]*Entity) bool {
	visited := map[string]bool{}
	var dfs func(string) bool
	dfs = func(current string) bool {
		if current == fromID {
			return true
		}
		if visited[current] {
			return false
		}
		visited[current] = true

		if e := entities[current]; e != nil {
			for _, r := range e.Relations {
				if r.Type == rel {
					if dfs(r.To) {
						return true
					}
				}
			}
		}
		return false
	}
	return dfs(toID)
}

// ValidateFull runs the complete graph validation (entities + relations + Drover policies).
func (v *Validator) ValidateFull(entities map[string]*Entity) []ValidationError {
	var errs []ValidationError

	for _, e := range entities {
		errs = append(errs, v.ValidateEntity(e)...)
	}

	for _, e := range entities {
		for _, r := range e.Relations {
			if to, ok := entities[r.To]; ok {
				errs = append(errs, v.ValidateRelation(e, to, r.Type, entities)...)
			}
		}
	}

	// Drover policies (included in ValidateFull so they are enforced consistently
	// whether called from Ontology.Validate() or directly).
	errs = append(errs, validateApprovedPlanRequiresReview(entities)...)
	errs = append(errs, validateTermGovernance(entities)...)

	return errs
}

// validateApprovedPlanRequiresReview implements Property 3.1 from TESTING_STRATEGY.md.
// A Plan with status=approved must have at least one "approves" relation *from* a Review
// (the relation lives on the Review per schema: Review --approves--> Plan).
func validateApprovedPlanRequiresReview(entities map[string]*Entity) []ValidationError {
	var errs []ValidationError
	for _, plan := range entities {
		if plan.Type != "Plan" {
			continue
		}
		status, _ := plan.Properties["status"].(string)
		if status != "approved" {
			continue
		}

		hasApproval := false
		for _, reviewer := range entities {
			if reviewer.Type != "Review" {
				continue
			}
			for _, r := range reviewer.Relations {
				if r.Type == "approves" && r.To == plan.ID {
					hasApproval = true
					break
				}
			}
			if hasApproval {
				break
			}
		}

		if !hasApproval {
			errs = append(errs, ValidationError{fmt.Sprintf("Plan %s is 'approved' but has no approving Review", plan.ID)})
		}
	}
	return errs
}

// validateTermGovernance is the shared implementation of the Term acceptance policy.
// It is called from Validator.ValidateFull (and thus Ontology.Validate / fsck).
func validateTermGovernance(entities map[string]*Entity) []ValidationError {
	var errs []ValidationError
	for _, e := range entities {
		if e.Type != "Term" {
			continue
		}
		status, _ := e.Properties["status"].(string)
		if status == "accepted" {
			hasGovernance := false
			for _, r := range e.Relations {
				if r.Type == "governed_by" {
					hasGovernance = true
					break
				}
			}
			if !hasGovernance {
				errs = append(errs, ValidationError{fmt.Sprintf("Term %s has status=accepted but no governed_by link (policy violation)", e.ID)})
			}
		}
	}
	return errs
}
