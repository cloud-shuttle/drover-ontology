//go:build go1.18

package ontology

import (
	"encoding/json"
	"testing"
)

// FuzzOperationRoundtrip exercises JSON serialization/deserialization
// of Operation and Entity types.
//
// These types are the backbone of the append-only log. Any corruption
// or loss of data here is catastrophic.
//
// Run with:
//   go test -fuzz=FuzzOperationRoundtrip -fuzztime=15s ./internal/ontology
func FuzzOperationRoundtrip(f *testing.F) {
	// Seed corpus with realistic operations
	seedOps := []Operation{
		{Op: "create", Entity: &Entity{ID: "e1", Type: "Bead", Properties: map[string]any{"title": "hello"}}},
		{Op: "update", ID: "e1", Set: map[string]any{"status": "done"}},
		{Op: "relate", From: "e1", Rel: "depends_on", To: "e2"},
		{Op: "invalidate", ID: "e1", ValidUntil: "2026-01-01T00:00:00Z"},
	}

	for _, op := range seedOps {
		data, _ := json.Marshal(op)
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var op Operation
		err := json.Unmarshal(data, &op)
		if err != nil {
			// Unmarshal failures are expected for garbage input
			return
		}

		// Roundtrip: must not lose critical fields
		roundtripped, err := json.Marshal(op)
		if err != nil {
			t.Fatalf("failed to remarshal valid Operation: %v", err)
		}

		var op2 Operation
		if err := json.Unmarshal(roundtripped, &op2); err != nil {
			t.Fatalf("failed to re-unmarshal: %v", err)
		}

		// Basic structural checks (we don't require perfect equality due to map ordering etc.)
		if op.Op != op2.Op {
			t.Errorf("Op changed after roundtrip: %s -> %s", op.Op, op2.Op)
		}
		if op.ID != op2.ID {
			t.Errorf("ID changed after roundtrip: %s -> %s", op.ID, op2.ID)
		}
		if op.From != op2.From || op.To != op2.To || op.Rel != op2.Rel {
			t.Error("relation fields changed after roundtrip")
		}
	})
}
