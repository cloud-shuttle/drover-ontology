package ontology

import (
	"strings"
	"testing"
)

// These tests exercise the Term governance policy that is documented in
// references/taxonomy-relations.md and the policies section of drover-schema.yaml:
//
//   - Anyone (human or agent) may create Terms with status="proposed".
//   - Promotion to status="accepted" requires at least one governed_by relation
//     (to a Person, Review, or Policy representing a Taxonomy Curator or formal acceptance).
//   - Attempting to accept without the governed_by link must be rejected by ValidateFull.
//
// The "must be performed by a designated curator" aspect is modeled by the curator
// being the one who adds the governed_by edge (social + process level); the mechanical
// guard we can enforce in the validator is the presence of the link.

func TestTermGovernance_ProposedIsAlwaysAllowed(t *testing.T) {
	ont := NewTestOntology(t)

	// Propose a new Term (anyone can do this).
	op := Operation{
		Op: "create",
		Entity: &Entity{
			ID:   "governance-term-1",
			Type: "Term",
			Properties: map[string]any{
				"label":      "Governance Term 1",
				"definition": "A term used to test governance rules.",
				"status":     "proposed",
			},
		},
	}
	if err := ont.Apply(op); err != nil {
		t.Fatalf("creating a Term with status=proposed should always succeed, got: %v", err)
	}

	// No governed_by yet — this is correct for the "proposed" state.
	// Explicit validation should also be clean.
	if errs := ont.Validate(); len(errs) != 0 {
		t.Fatalf("Validate after proposing a Term should have no errors, got: %v", errs)
	}
}

func TestTermGovernance_AcceptAfterAddingGovernedBy(t *testing.T) {
	ont := NewTestOntology(t)

	// 1. Curator (a Person who will act as Taxonomy Curator for this test).
	curator := &Entity{
		ID:   "curator-peter",
		Type: "Person",
		Properties: map[string]any{
			"name": "Peter (test curator)",
		},
	}
	if err := ont.Apply(Operation{Op: "create", Entity: curator}); err != nil {
		t.Fatalf("failed to create curator person: %v", err)
	}

	// (Optional but realistic) model the role: the Person is tagged_with the "Taxonomy Curator" Term.
	// The role Term itself must also satisfy governance (it is accepted in the real graph).
	role := &Entity{
		ID:   "Taxonomy Curator",
		Type: "Term",
		Properties: map[string]any{
			"label":      "Taxonomy Curator",
			"definition": "Designated acceptor for Terms in the drover-org taxonomy.",
			"status":     "accepted",
		},
	}
	_ = ont.Apply(Operation{Op: "create", Entity: role})
	// Give the role Term its own governance link so the policy is satisfied.
	_ = ont.Apply(Operation{Op: "relate", From: "Taxonomy Curator", Rel: "governed_by", To: "curator-peter"})
	_ = ont.Apply(Operation{Op: "relate", From: "curator-peter", Rel: "tagged_with", To: "Taxonomy Curator"})

	// 2. Propose a new Term.
	termID := "governed-term"
	term := &Entity{
		ID:   termID,
		Type: "Term",
		Properties: map[string]any{
			"label":      "Governed Term",
			"definition": "This term will be properly governed before acceptance.",
			"status":     "proposed",
		},
	}
	if err := ont.Apply(Operation{Op: "create", Entity: term}); err != nil {
		t.Fatalf("failed to propose term: %v", err)
	}

	// 3. Curator records accountability.
	if err := ont.Apply(Operation{
		Op:   "relate",
		From: termID,
		Rel:  "governed_by",
		To:   "curator-peter",
	}); err != nil {
		t.Fatalf("failed to add governed_by link: %v", err)
	}

	// 4. Now the curator may promote to accepted.
	if err := ont.Apply(Operation{
		Op:  "update",
		ID:  termID,
		Set: map[string]any{"status": "accepted"},
	}); err != nil {
		t.Fatalf("failed to promote term to accepted after adding governed_by: %v", err)
	}

	// Validation must now pass.
	if errs := ont.Validate(); len(errs) != 0 {
		t.Fatalf("expected clean validation after proper governed_by + accept, got: %v", errs)
	}

	// Sanity: the term is visible with the correct status.
	e, ok := ont.Get(termID)
	if !ok || e.Properties["status"] != "accepted" {
		t.Fatalf("expected term to be accepted, got: %+v", e)
	}
}

func TestTermGovernance_RejectAcceptWithoutGovernedBy(t *testing.T) {
	ont := NewTestOntology(t)

	// Propose a Term.
	termID := "ungoverned-term"
	term := &Entity{
		ID:   termID,
		Type: "Term",
		Properties: map[string]any{
			"label":      "Ungoverned Term",
			"definition": "This term tries to become accepted without any governed_by link.",
			"status":     "proposed",
		},
	}
	if err := ont.Apply(Operation{Op: "create", Entity: term}); err != nil {
		t.Fatalf("failed to create proposed term: %v", err)
	}

	// Attempt to accept without any governed_by link.
	if err := ont.Apply(Operation{
		Op:  "update",
		ID:  termID,
		Set: map[string]any{"status": "accepted"},
	}); err != nil {
		// Some implementations may reject at Apply time; others accept the mutation
		// and only fail on explicit Validate / fsck. Either is acceptable for the policy
		// as long as Validate surfaces the violation.
	}

	// The key assertion: full validation must report the governance violation.
	errs := ont.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, termID) || strings.Contains(e.Message, "governed_by") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Validate() to report governance violation for accepting Term without governed_by, got: %v", errs)
	}
}
