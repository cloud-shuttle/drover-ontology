package ontology

import "time"

// applyOperation is the single canonical implementation of the Operation folding
// logic (the "OperationReducer"). It is the pure(ish) transformation:
//   current entities map + Operation  -->  mutated entities map
//
// This is now the *one* place that knows how to apply create/update/relate/unrelate/
// invalidate/tombstone, including:
//   - copying Entity on create + ensuring Relations slice
//   - property merge + Updated bump on update
//   - relation append (with props) / filter on relate/unrelate
//   - ValidUntil parse (RFC3339 or raw fallback) + Updated on invalidate
//   - status=tombstoned + *always* tombstone_reason (even "") on tombstone
//
// Both the authoritative replay path (Log.LoadAll) and the live Apply path
// (Ontology.applyToMemory) delegate to this. This directly addresses Candidate 01
// from the architecture review: eliminates the duplicated switches that were the
// root cause of applyToMemory vs log replay drift (see regression seeds and
// TestLogReplayEquivalence). It increases Locality (mutation semantics in one
// place owned by the Ontology Module) and Leverage (callers/tests get consistent
// behavior from small surface).
//
// The map is mutated in place for compatibility with existing call sites and
// to avoid unnecessary allocations on the hot path. The "now" for Updated
// timestamps is taken at fold time (replay time for LoadAll, apply time for live).
//
// This is the core of the deepened internal/ontology module for operation
// semantics.
func applyOperation(entities map[string]*Entity, op Operation) {
	now := time.Now().UTC()
	switch op.Op {
	case "create":
		if op.Entity != nil {
			e := *op.Entity // copy
			if e.Relations == nil {
				e.Relations = []Relation{}
			}
			entities[e.ID] = &e
		}

	case "update":
		if e, ok := entities[op.ID]; ok {
			if op.Set != nil {
				for k, v := range op.Set {
					e.Properties[k] = v
				}
			}
			e.Updated = now
		}

	case "relate":
		if e, ok := entities[op.From]; ok {
			rel := Relation{Type: op.Rel, To: op.To}
			if op.Properties != nil {
				rel.Properties = op.Properties
			}
			e.Relations = append(e.Relations, rel)
		}

	case "unrelate":
		if e, ok := entities[op.From]; ok {
			filtered := make([]Relation, 0, len(e.Relations))
			for _, r := range e.Relations {
				if !(r.Type == op.Rel && r.To == op.To) {
					filtered = append(filtered, r)
				}
			}
			e.Relations = filtered
		}

	case "invalidate":
		if e, ok := entities[op.ID]; ok {
			if op.ValidUntil != "" {
				if t, err := time.Parse(time.RFC3339, op.ValidUntil); err == nil {
					e.ValidUntil = &t
				} else {
					// Preserve raw for debugging / strict recovery (matches prior LoadAll)
					e.Properties["valid_until_raw"] = op.ValidUntil
				}
			}
			e.Updated = now
		}

	case "tombstone":
		if e, ok := entities[op.ID]; ok {
			e.Properties["status"] = "tombstoned"
			e.Properties["tombstone_reason"] = op.Reason // always set (may be ""); unifies with Log replay
			// Do not unconditionally bump Updated here (preserves prior LoadAll behavior for tombstone).
			// Updated is bumped for create/update/invalidate/relate surfaces.
		}
	}
}
