//go:build rlm

package main

import (
	"bytes"
	"sync"
	"testing"

	"github.com/XiaoConstantine/rlm-go/pkg/parsing"
)

func TestMarkdownParserProperties(t *testing.T) {
	// Property: Any standard code block format is successfully parsed, and doesn't crash the parser.
	testCases := []struct {
		name     string
		input    string
		expected int
	}{
		{"standard block", "```go\nfmt.Println(1)\n```", 1},
		{"multiple blocks", "```go\nfmt.Println(1)\n```\nSome text\n```go\nfmt.Println(2)\n```", 2},
		{"various spacing", "```go\nfmt.Println(1)\n```  ", 1},
		{"empty block", "```go\n```", 0},
		{"block with text", "Hello\n```go\nfmt.Println(1)\n```\nWorld", 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			blocks := parsing.FindCodeBlocks(tc.input)
			if len(blocks) != tc.expected {
				t.Errorf("expected %d blocks, got: %d", tc.expected, len(blocks))
			}
		})
	}
}

func FuzzCaptureWriter(f *testing.F) {
	f.Add([]byte("initial fuzz input"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var target bytes.Buffer
		cw := &captureWriter{target: &target}

		// Verify concurrency safety under random concurrent writes
		var wg sync.WaitGroup
		concurrency := 5
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = cw.Write(data)
			}()
		}
		wg.Wait()

		// Verify data size matching in target and buffer
		if len(cw.String()) != len(target.String()) {
			t.Errorf("size mismatch: buffer %d, target %d", len(cw.String()), len(target.String()))
		}
	})
}
