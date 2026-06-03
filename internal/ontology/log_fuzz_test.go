//go:build go1.18

package ontology

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzLogLoadAll exercises the JSONL log parser with arbitrary input.
// This is the highest-priority fuzz target because the append-only log
// is the source of truth for the entire ontology.
//
// Run with:
//   go test -fuzz=FuzzLogLoadAll -fuzztime=30s ./internal/ontology
func FuzzLogLoadAll(f *testing.F) {
	// Seed corpus with some realistic + edge cases
	f.Add([]byte(`{"op":"create","entity":{"id":"seed1","type":"Bead","properties":{"title":"x"}}}` + "\n"))
	f.Add([]byte(""))
	f.Add([]byte("\n\n"))
	f.Add([]byte(`{"op":"invalid"`))
	f.Add([]byte(`{"op":"create","entity":{"id":"bad","type":"Bead"}`)) // truncated

	f.Fuzz(func(t *testing.T, data []byte) {
		tmp := t.TempDir()
		path := filepath.Join(tmp, "fuzz.jsonl")

		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Skip("failed to write fuzz input")
		}

		l := &Log{path: path}
		ents, ops, err := l.LoadAll()

		// We don't assert much — the goal is to find panics, infinite loops,
		// or corrupt state. Any non-panic outcome is acceptable for garbage input.
		_ = ents
		_ = ops
		_ = err
	})
}
