package ontology

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestAcyclicitySoundness implements Property 2.1 from TESTING_STRATEGY.md.
func TestAcyclicitySoundness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		entities := generateRandomGraph(rt)

		fromID := pickEntityID(entities, rt)
		toID := pickEntityID(entities, rt)
		rel := rapid.SampledFrom([]string{"depends_on", "blocks"}).Draw(rt, "rel")

		ourResult := createsCycle(fromID, toID, rel, entities)
		refResult := wouldCreateCycleReference(fromID, toID, rel, entities)

		if ourResult != refResult {
			rt.Fatalf("createsCycle mismatch for rel=%s from=%s to=%s\nour=%v ref=%v\n%s",
				rel, fromID, toID, ourResult, refResult, summarizeGraph(entities))
		}
	})
}

func wouldCreateCycleReference(fromID, toID, rel string, entities map[string]*Entity) bool {
	graph := make(map[string][]string, len(entities))
	for id, e := range entities {
		for _, r := range e.Relations {
			if r.Type == rel {
				graph[id] = append(graph[id], r.To)
			}
		}
	}
	graph[fromID] = append(graph[fromID], toID)

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
		for _, next := range graph[current] {
			if dfs(next) {
				return true
			}
		}
		return false
	}
	return dfs(toID)
}

func generateRandomGraph(t *rapid.T) map[string]*Entity {
	n := rapid.IntRange(2, 10).Draw(t, "numEntities")
	entities := make(map[string]*Entity, n)
	ids := make([]string, n)

	for i := 0; i < n; i++ {
		id := rapid.StringN(3, 10, 14).Draw(t, "id")
		ids[i] = id
		entities[id] = &Entity{ID: id, Type: "Task"}
	}

	numRels := rapid.IntRange(0, n+2).Draw(t, "numRels")
	for i := 0; i < numRels; i++ {
		fromIdx := rapid.IntRange(0, n-1).Draw(t, "from")
		toIdx := rapid.IntRange(0, n-1).Draw(t, "to")
		if fromIdx == toIdx {
			continue
		}
		from := ids[fromIdx]
		to := ids[toIdx]
		relType := rapid.SampledFrom([]string{"depends_on", "blocks"}).Draw(t, "relType")

		entities[from].Relations = append(entities[from].Relations, Relation{
			Type: relType,
			To:   to,
		})
	}
	return entities
}

func pickEntityID(entities map[string]*Entity, t *rapid.T) string {
	ids := make([]string, 0, len(entities))
	for id := range entities {
		ids = append(ids, id)
	}
	return rapid.SampledFrom(ids).Draw(t, "pickID")
}

func summarizeGraph(entities map[string]*Entity) string {
	totalRels := 0
	for _, e := range entities {
		totalRels += len(e.Relations)
	}
	return fmt.Sprintf("entities=%d relations=%d", len(entities), totalRels)
}

// TestInvalidatedEntitiesInvisible implements Property 5.1.
func TestInvalidatedEntitiesInvisible(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		ont := NewTestOntology(t)

		numEntities := rapid.IntRange(2, 6).Draw(rt, "numEntities")
		createdIDs := make([]string, 0, numEntities)

		for i := 0; i < numEntities; i++ {
			e := Entity{
				ID:   fmt.Sprintf("ent_%d_%s", i, rapid.StringN(4, 8, 10).Draw(rt, "suffix")),
				Type: "Task",
				Properties: map[string]any{
					"title": rapid.StringN(5, 20, 30).Draw(rt, "title"),
				},
			}
			op := Operation{Op: "create", Entity: &e, Ts: time.Now().UTC()}
			if err := ont.Apply(op); err != nil {
				rt.Fatalf("create failed: %v", err)
			}
			createdIDs = append(createdIDs, e.ID)
		}

		// Invalidate a subset with past ValidUntil
		numToInvalidate := rapid.IntRange(0, len(createdIDs)).Draw(rt, "numToInvalidate")
		toInvalidate := make([]string, 0, numToInvalidate)
		for i := 0; i < numToInvalidate; i++ {
			idx := rapid.IntRange(0, len(createdIDs)-1).Draw(rt, "idx")
			id := createdIDs[idx]
			already := false
			for _, existing := range toInvalidate {
				if existing == id {
					already = true
				}
			}
			if !already {
				toInvalidate = append(toInvalidate, id)
				invOp := Operation{
					Op:         "invalidate",
					ID:         id,
					ValidUntil: time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
					Ts:         time.Now().UTC(),
				}
				_ = ont.Apply(invOp)
			}
		}

		// HybridSearch is the main public API that filters invalidated entities
		results, _ := ont.HybridSearch("title", 20, 0.7)
		for _, id := range toInvalidate {
			for _, r := range results {
				if r.ID == id {
					rt.Fatalf("invalidated entity %s still appears in HybridSearch", id)
				}
			}
		}

		// Note: As of writing, ont.All() and ont.Get() return raw entities including invalidated ones.
		// HybridSearch is the primary place where temporal filtering is applied.
	})
}

// TestApprovedPlanRequiresReview implements Property 3.1 from TESTING_STRATEGY.md
// (and the acceptance criteria for bea_ont_006).
//
// After any sequence of operations, a Plan with status=="approved" must cause
// Validate() / ValidateFull() to report a violation *unless* there is at least
// one "approves" relation from a Review entity to that Plan.
func TestApprovedPlanRequiresReview(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		ont := NewTestOntology(t)

		// Generate a small number of Plans and Reviews
		numPlans := rapid.IntRange(1, 4).Draw(rt, "numPlans")
		numReviews := rapid.IntRange(0, 3).Draw(rt, "numReviews")

		planIDs := make([]string, 0, numPlans)
		reviewIDs := make([]string, 0, numReviews)

		for i := 0; i < numPlans; i++ {
			id := fmt.Sprintf("plan_%d_%s", i, rapid.StringN(3, 6, 8).Draw(rt, "pid"))
			e := Entity{
				ID:   id,
				Type: "Plan",
				Properties: map[string]any{
					"title":  rapid.StringN(3, 20, 30).Draw(rt, "title"),
					"kind":   "feature",
					"status": "draft",
				},
			}
			_ = ont.Apply(Operation{Op: "create", Entity: &e, Ts: time.Now().UTC()})
			planIDs = append(planIDs, id)
		}

		for i := 0; i < numReviews; i++ {
			id := fmt.Sprintf("rev_%d_%s", i, rapid.StringN(3, 6, 8).Draw(rt, "rid"))
			e := Entity{
				ID:   id,
				Type: "Review",
				Properties: map[string]any{
					"kind":     "asdlc",
					"decision": rapid.SampledFrom([]string{"go", "conditional_go", "no_go"}).Draw(rt, "decision"),
				},
			}
			_ = ont.Apply(Operation{Op: "create", Entity: &e, Ts: time.Now().UTC()})
			reviewIDs = append(reviewIDs, id)
		}

		// Randomly add some "approves" relations (from Review -> Plan)
		maxApprovals := numReviews
		if numPlans < numReviews {
			maxApprovals = numPlans
		}
		numApprovals := rapid.IntRange(0, maxApprovals+1).Draw(rt, "numApprovals")
		approvedPlansWithReview := map[string]bool{}
		for i := 0; i < numApprovals && len(reviewIDs) > 0 && len(planIDs) > 0; i++ {
			rid := reviewIDs[rapid.IntRange(0, len(reviewIDs)-1).Draw(rt, "ridx")]
			pid := planIDs[rapid.IntRange(0, len(planIDs)-1).Draw(rt, "pidx")]
			_ = ont.Apply(Operation{
				Op:   "relate",
				From: rid,
				Rel:  "approves",
				To:   pid,
				Ts:   time.Now().UTC(),
			})
			approvedPlansWithReview[pid] = true
		}

		// Randomly promote some Plans to "approved"
		numToApprove := rapid.IntRange(0, numPlans).Draw(rt, "numToApprove")
		for i := 0; i < numToApprove; i++ {
			pid := planIDs[rapid.IntRange(0, len(planIDs)-1).Draw(rt, "pidx")]
			_ = ont.Apply(Operation{
				Op:  "update",
				ID:  pid,
				Set: map[string]any{"status": "approved"},
				Ts:  time.Now().UTC(),
			})
		}

		// Now ask the policy checker (via the public API we surface on Ontology)
		errs := ont.Validate()

		// For every Plan that is now approved, we must have a violation *unless*
		// we recorded an approving Review for it.
		for _, pid := range planIDs {
			plan, ok := ont.Get(pid)
			if !ok {
				continue
			}
			status, _ := plan.Properties["status"].(string)
			if status != "approved" {
				continue
			}

			hasReview := approvedPlansWithReview[pid]
			hasViolation := false
			for _, e := range errs {
				if strings.Contains(e.Message, pid) && strings.Contains(e.Message, "approved") {
					hasViolation = true
					break
				}
			}

			if hasReview && hasViolation {
				rt.Fatalf("Plan %s has an approving Review but Validate reported a violation: %v", pid, errs)
			}
			if !hasReview && !hasViolation {
				rt.Fatalf("Plan %s is approved with no approving Review but Validate reported no violation", pid)
			}
		}
	})
}
