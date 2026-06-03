//go:build rlm

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOpenAIMarshalling(t *testing.T) {
	// Verify OpenAI JSON structure is correctly parsed
	rawResponse := `{
		"choices": [
			{
				"message": {
					"content": "fmt.Println(\"success\")"
				}
			}
		]
	}`

	var resp openaiResponse
	if err := json.Unmarshal([]byte(rawResponse), &resp); err != nil {
		t.Fatalf("failed to unmarshal OpenAI response: %v", err)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got: %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != `fmt.Println("success")` {
		t.Errorf("expected content 'fmt.Println(\"success\")', got: %s", resp.Choices[0].Message.Content)
	}
}

func TestAnthropicMarshalling(t *testing.T) {
	// Verify Anthropic JSON structure is correctly parsed
	rawResponse := `{
		"content": [
			{
				"type": "text",
				"text": "fmt.Println(\"success\")"
			}
		]
	}`

	var resp anthropicResponse
	if err := json.Unmarshal([]byte(rawResponse), &resp); err != nil {
		t.Fatalf("failed to unmarshal Anthropic response: %v", err)
	}

	if len(resp.Content) != 1 {
		t.Fatalf("expected 1 content block, got: %d", len(resp.Content))
	}
	if resp.Content[0].Text != `fmt.Println("success")` {
		t.Errorf("expected text 'fmt.Println(\"success\")', got: %s", resp.Content[0].Text)
	}
}

func TestRLMHistoryFeedbackLoop(t *testing.T) {
	// Simulate the execution feedback capturing loop
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	stdoutWriter := &captureWriter{target: &stdout}
	stderrWriter := &captureWriter{target: &stderr}

	// 1. Reset writers
	stdoutWriter.Reset()
	stderrWriter.Reset()

	// 2. Simulate REPL writes
	fmt.Fprint(stdoutWriter, "Makefile README.md config.yaml\n")
	fmt.Fprint(stderrWriter, "warning: deprecated syntax\n")

	// 3. Capture feedback logic matching the main loop
	output := stdoutWriter.String()
	errOutput := stderrWriter.String()

	var history []string
	if len(output) > 0 || len(errOutput) > 0 {
		var feedback strings.Builder
		feedback.WriteString("Execution feedback from the Go REPL:\n")
		if len(output) > 0 {
			feedback.WriteString(fmt.Sprintf("=== STDOUT ===\n%s\n", output))
		}
		if len(errOutput) > 0 {
			feedback.WriteString(fmt.Sprintf("=== STDERR ===\n%s\n", errOutput))
		}
		history = append(history, feedback.String())
	}

	// 4. Verification
	if len(history) != 1 {
		t.Fatalf("expected 1 history item, got: %d", len(history))
	}

	feedbackStr := history[0]
	if !strings.Contains(feedbackStr, "Execution feedback from the Go REPL:") {
		t.Errorf("missing header in feedback: %s", feedbackStr)
	}
	if !strings.Contains(feedbackStr, "Makefile README.md config.yaml") {
		t.Errorf("missing stdout in feedback: %s", feedbackStr)
	}
	if !strings.Contains(feedbackStr, "warning: deprecated syntax") {
		t.Errorf("missing stderr in feedback: %s", feedbackStr)
	}
}
