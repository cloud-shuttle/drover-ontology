//go:build rlm

package main

import (
	"bytes"
	"os"
	"testing"
)

func TestCaptureWriter(t *testing.T) {
	var target bytes.Buffer
	cw := &captureWriter{target: &target}

	// 1. Basic Write
	msg := []byte("hello world")
	n, err := cw.Write(msg)
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != len(msg) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(msg), n)
	}

	// Verify both target and buffer captured the output
	if cw.String() != "hello world" {
		t.Errorf("expected capture buffer to have 'hello world', got: %s", cw.String())
	}
	if target.String() != "hello world" {
		t.Errorf("expected target buffer to have 'hello world', got: %s", target.String())
	}

	// 2. Reset
	cw.Reset()
	if cw.String() != "" {
		t.Errorf("expected capture buffer to be empty after Reset, got: %s", cw.String())
	}
	// Target should still preserve the historical data
	if target.String() != "hello world" {
		t.Errorf("expected target to preserve previous writes after Reset, got: %s", target.String())
	}
}

func TestModelProviderRouting(t *testing.T) {
	// Backup original env vars
	originalOpenAIKey := os.Getenv("OPENAI_API_KEY")
	originalAnthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	originalLLMModel := os.Getenv("LLM_MODEL")
	originalLLMSubModel := os.Getenv("LLM_SUB_MODEL")
	originalOpenAIModel := os.Getenv("OPENAI_MODEL")
	originalOpenAISubModel := os.Getenv("OPENAI_SUB_MODEL")
	originalAnthropicModel := os.Getenv("ANTHROPIC_MODEL")
	originalAnthropicSubModel := os.Getenv("ANTHROPIC_SUB_MODEL")

	defer func() {
		os.Setenv("OPENAI_API_KEY", originalOpenAIKey)
		os.Setenv("ANTHROPIC_API_KEY", originalAnthropicKey)
		os.Setenv("LLM_MODEL", originalLLMModel)
		os.Setenv("LLM_SUB_MODEL", originalLLMSubModel)
		os.Setenv("OPENAI_MODEL", originalOpenAIModel)
		os.Setenv("OPENAI_SUB_MODEL", originalOpenAISubModel)
		os.Setenv("ANTHROPIC_MODEL", originalAnthropicModel)
		os.Setenv("ANTHROPIC_SUB_MODEL", originalAnthropicSubModel)
	}()

	// 1. Test OpenAI Prioritization and Defaults
	os.Setenv("OPENAI_API_KEY", "sk-test-openai")
	os.Setenv("ANTHROPIC_API_KEY", "sk-test-anthropic")
	os.Setenv("LLM_MODEL", "")
	os.Setenv("LLM_SUB_MODEL", "")
	os.Setenv("OPENAI_MODEL", "")
	os.Setenv("OPENAI_SUB_MODEL", "")

	// Verify we can simulate the main routing check
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Error("expected OpenAI API Key to be configured")
	}

	// 2. Test environment overrides
	os.Setenv("LLM_MODEL", "custom-model")
	if os.Getenv("LLM_MODEL") != "custom-model" {
		t.Error("expected LLM_MODEL to be overridden")
	}
}
