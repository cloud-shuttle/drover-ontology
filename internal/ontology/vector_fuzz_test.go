//go:build go1.18

package ontology

import (
	"math"
	"testing"
)

// FuzzVectorMath exercises generateDummyVector and cosineSimilarity
// with adversarial inputs.
//
// These are pure functions that feed directly into HybridSearch and
// automatic embedding. They are excellent fuzz targets.
//
// Run with:
//   go test -fuzz=FuzzVectorMath -fuzztime=20s ./internal/ontology
func FuzzVectorMath(f *testing.F) {
	// Seed corpus entries must match the Fuzz function signature:
	// (string, []byte, []byte)
	f.Add("normal text", []byte{}, []byte{})
	f.Add("", []byte{0x00}, []byte{0xff})
	f.Add(string(make([]byte, 5000)), []byte{}, []byte{})
	f.Add("unicode: 日本語 \u0000 \uffff", []byte{1, 2, 3}, []byte{4, 5, 6})

	// Seed some vector patterns via the byte parameters
	addVec := func(v []float32) {
		b := mustMarshalFloats(v)
		f.Add("vecseed", b, b)
	}
	addVec([]float32{1, 0, 0})
	addVec([]float32{0, 0, 0})
	addVec(make([]float32, 384))

	f.Fuzz(func(t *testing.T, text string, rawA, rawB []byte) {
		// Turn raw bytes into float32 slices (best effort)
		a := bytesToFloat32Slice(rawA)
		b := bytesToFloat32Slice(rawB)

		// Should never panic
		_ = generateDummyVector(text)

		if len(a) > 0 && len(b) > 0 {
			score := cosineSimilarity(a, b)

			// Basic sanity: result should be in [-1, 1] or NaN (for degenerate cases)
			if !math.IsNaN(score) {
				if score < -1.0 || score > 1.0 {
					t.Errorf("cosineSimilarity out of range: %f", score)
				}
			}
		}
	})
}

// helpers (kept local to the fuzz file)
func bytesToFloat32Slice(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	// Very rough conversion: treat every 4 bytes as a float32 (little endian-ish)
	out := make([]float32, 0, len(b)/4)
	for i := 0; i+3 < len(b); i += 4 {
		bits := uint32(b[i]) | uint32(b[i+1])<<8 | uint32(b[i+2])<<16 | uint32(b[i+3])<<24
		out = append(out, math.Float32frombits(bits))
	}
	return out
}

func mustMarshalFloats(v []float32) []byte {
	// Simple stable representation for seeding corpus
	b := make([]byte, len(v)*4)
	for i, f := range v {
		bits := math.Float32bits(f)
		b[i*4] = byte(bits)
		b[i*4+1] = byte(bits >> 8)
		b[i*4+2] = byte(bits >> 16)
		b[i*4+3] = byte(bits >> 24)
	}
	return b
}
