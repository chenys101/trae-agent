// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package llm

import (
	"encoding/json"
	"testing"
)

func TestLLMMessageCreation(t *testing.T) {
	msg := LLMMessage{
		Role:    "system",
		Content: "You are a helpful assistant.",
	}
	if msg.Role != "system" {
		t.Errorf("expected role 'system', got '%s'", msg.Role)
	}
	if msg.Content != "You are a helpful assistant." {
		t.Errorf("unexpected content: %s", msg.Content)
	}
}

func TestLLMMessageWithToolCalls(t *testing.T) {
	msg := LLMMessage{
		Role:    "assistant",
		Content: "",
		ToolCalls: []ToolCallInfo{
			{
				Name:   "read_file",
				CallID: "call_123",
				Arguments: map[string]any{
					"path": "/tmp/test.txt",
				},
			},
		},
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Name != "read_file" {
		t.Errorf("expected tool name 'read_file', got '%s'", msg.ToolCalls[0].Name)
	}
	if msg.ToolCalls[0].CallID != "call_123" {
		t.Errorf("expected call ID 'call_123', got '%s'", msg.ToolCalls[0].CallID)
	}
}

func TestLLMMessageToolResult(t *testing.T) {
	msg := LLMMessage{
		Role:       "tool",
		ToolCallID: "call_123",
		ToolName:   "read_file",
		ToolResult: "file contents here",
	}
	if msg.Role != "tool" {
		t.Errorf("expected role 'tool', got '%s'", msg.Role)
	}
	if msg.ToolCallID != "call_123" {
		t.Errorf("expected tool call ID 'call_123', got '%s'", msg.ToolCallID)
	}
	if msg.ToolResult != "file contents here" {
		t.Errorf("unexpected tool result: %s", msg.ToolResult)
	}
}

func TestLLMMessageWithImages(t *testing.T) {
	msg := LLMMessage{
		Role:    "user",
		Content: "What is in this image?",
		Images: []ImageContent{
			{Type: "image_url", URL: "https://example.com/image.png"},
		},
	}
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(msg.Images))
	}
	if msg.Images[0].Type != "image_url" {
		t.Errorf("expected type 'image_url', got '%s'", msg.Images[0].Type)
	}
}

func TestLLMResponseCreation(t *testing.T) {
	resp := LLMResponse{
		Content: "Hello!",
		Usage: LLMUsage{
			InputTokens:  10,
			OutputTokens: 5,
		},
		StopReason: "end_turn",
	}
	if resp.Content != "Hello!" {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input tokens 10, got %d", resp.Usage.InputTokens)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop reason 'end_turn', got '%s'", resp.StopReason)
	}
}

func TestModelConfigCreation(t *testing.T) {
	cfg := ModelConfig{
		Model:            "claude-3-5-sonnet",
		Provider:         "anthropic",
		MaxTokens:        4096,
		Temperature:      0.7,
		TopP:             0.9,
		TopK:             0,
		MaxRetries:       3,
		ParallelToolCalls: true,
		APIKey:           "sk-test",
		BaseURL:          "https://api.anthropic.com",
	}
	if cfg.Model != "claude-3-5-sonnet" {
		t.Errorf("unexpected model: %s", cfg.Model)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", cfg.Temperature)
	}
	if !cfg.ParallelToolCalls {
		t.Error("expected ParallelToolCalls to be true")
	}
}

func TestLLMMessageJSONSerialization(t *testing.T) {
	msg := LLMMessage{
		Role:    "user",
		Content: "Hello",
		Images: []ImageContent{
			{Type: "image_url", URL: "https://example.com/img.png"},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded LLMMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Role != msg.Role {
		t.Errorf("expected role '%s', got '%s'", msg.Role, decoded.Role)
	}
	if decoded.Content != msg.Content {
		t.Errorf("expected content '%s', got '%s'", msg.Content, decoded.Content)
	}
	if len(decoded.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(decoded.Images))
	}
	if decoded.Images[0].URL != msg.Images[0].URL {
		t.Errorf("expected URL '%s', got '%s'", msg.Images[0].URL, decoded.Images[0].URL)
	}
}

func TestLLMResponseJSONSerialization(t *testing.T) {
	resp := LLMResponse{
		Content: "result",
		ToolCalls: []ToolCallInfo{
			{Name: "bash", CallID: "call_1", Arguments: map[string]any{"command": "ls"}},
		},
		Usage:      LLMUsage{InputTokens: 100, OutputTokens: 50},
		StopReason: "tool_use",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded LLMResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if decoded.Content != resp.Content {
		t.Errorf("expected content '%s', got '%s'", resp.Content, decoded.Content)
	}
	if decoded.StopReason != resp.StopReason {
		t.Errorf("expected stop_reason '%s', got '%s'", resp.StopReason, decoded.StopReason)
	}
	if decoded.Usage.InputTokens != resp.Usage.InputTokens {
		t.Errorf("expected input_tokens %d, got %d", resp.Usage.InputTokens, decoded.Usage.InputTokens)
	}
	if len(decoded.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(decoded.ToolCalls))
	}
	if decoded.ToolCalls[0].Name != "bash" {
		t.Errorf("expected tool name 'bash', got '%s'", decoded.ToolCalls[0].Name)
	}
}

func TestNewClientUnsupported(t *testing.T) {
	client, err := NewClient("unsupported", "sk-test", "https://api.example.com")
	if client != nil {
		t.Error("expected nil client")
	}
	if err == nil {
		t.Error("expected error")
	}
}

func TestNewClientAnthropic(t *testing.T) {
	client, err := NewClient("anthropic", "sk-test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewClientOpenAI(t *testing.T) {
	client, err := NewClient("openai", "sk-test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
