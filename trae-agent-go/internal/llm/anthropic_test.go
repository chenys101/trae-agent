// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bytedance/trae-agent-go/internal/tool"
)

// anthropicMockTool 实现 tool.Tool 接口用于测试。
type anthropicMockTool struct {
	name        string
	description string
	parameters  []tool.ToolParameter
}

func (m *anthropicMockTool) GetName() string                    { return m.name }
func (m *anthropicMockTool) GetDescription() string              { return m.description }
func (m *anthropicMockTool) GetParameters() []tool.ToolParameter { return m.parameters }
func (m *anthropicMockTool) Execute(ctx context.Context, args map[string]any) (tool.ToolResult, error) {
	return tool.ToolResult{}, nil
}
func (m *anthropicMockTool) GetInputSchema() map[string]any {
	return tool.GetInputSchema(m)
}

func newAnthropicMockTool() *anthropicMockTool {
	return &anthropicMockTool{
		name:        "test_tool",
		description: "A test tool",
		parameters: []tool.ToolParameter{
			{Name: "query", Type: "string", Description: "The query", Required: true},
		},
	}
}

func defaultModelConfig() ModelConfig {
	return ModelConfig{
		Model:       "claude-3-5-sonnet-20241022",
		MaxTokens:   1024,
		MaxRetries:  0, // 测试时不重试
		Temperature: 0.7,
		APIKey:      "test-api-key",
	}
}

func TestAnthropicClient_NewClient(t *testing.T) {
	client := NewAnthropicClient("test-key", "")
	if client.baseURL != defaultAnthropicBaseURL {
		t.Errorf("expected default baseURL %s, got %s", defaultAnthropicBaseURL, client.baseURL)
	}
	if client.apiKey != "test-key" {
		t.Errorf("expected apiKey test-key, got %s", client.apiKey)
	}

	client2 := NewAnthropicClient("test-key", "https://custom.api.com")
	if client2.baseURL != "https://custom.api.com" {
		t.Errorf("expected custom baseURL, got %s", client2.baseURL)
	}
}

func TestAnthropicClient_NormalResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/messages" {
			t.Errorf("expected /messages, got %s", r.URL.Path)
		}

		if r.Header.Get("x-api-key") != "test-api-key" {
			t.Errorf("expected x-api-key header, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version header, got %s", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("content-type") != "application/json" {
			t.Errorf("expected content-type header, got %s", r.Header.Get("content-type"))
		}

		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// 验证 system 消息被提取到顶层
		if reqBody.System != "You are a helpful assistant" {
			t.Errorf("expected system message in top-level field, got %s", reqBody.System)
		}

		// 验证消息格式
		if len(reqBody.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "user" {
			t.Errorf("expected first message role user, got %s", reqBody.Messages[0].Role)
		}
		if reqBody.Messages[1].Role != "assistant" {
			t.Errorf("expected second message role assistant, got %s", reqBody.Messages[1].Role)
		}

		resp := anthropicResponse{
			ID:    "msg_test",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-5-sonnet-20241022",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Hello! How can I help you?"},
			},
			StopReason: "end_turn",
			Usage: anthropicUsage{
				InputTokens:  10,
				OutputTokens: 20,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key", server.URL)
	cfg := defaultModelConfig()
	messages := []LLMMessage{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	resp, err := client.Chat(context.Background(), messages, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Hello! How can I help you?" {
		t.Errorf("expected content 'Hello! How can I help you?', got %s", resp.Content)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got %s", resp.StopReason)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("expected output_tokens 20, got %d", resp.Usage.OutputTokens)
	}
	if len(resp.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.ToolCalls))
	}
}

func TestAnthropicClient_ToolCallResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// 验证工具定义格式
		if len(reqBody.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(reqBody.Tools))
		}
		if reqBody.Tools[0].Name != "test_tool" {
			t.Errorf("expected tool name 'test_tool', got %s", reqBody.Tools[0].Name)
		}
		if reqBody.Tools[0].Description != "A test tool" {
			t.Errorf("expected tool description 'A test tool', got %s", reqBody.Tools[0].Description)
		}
		if reqBody.Tools[0].InputSchema == nil {
			t.Error("expected tool input_schema to be set")
		}

		resp := anthropicResponse{
			ID:    "msg_test",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-5-sonnet-20241022",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Let me look that up."},
				{Type: "tool_use", ID: "call_123", Name: "test_tool", Input: map[string]any{"query": "hello"}},
			},
			StopReason: "tool_use",
			Usage: anthropicUsage{
				InputTokens:  15,
				OutputTokens: 30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key", server.URL)
	cfg := defaultModelConfig()
	messages := []LLMMessage{
		{Role: "user", Content: "Search for hello"},
	}

	tools := []tool.Tool{newAnthropicMockTool()}
	resp, err := client.Chat(context.Background(), messages, cfg, tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Let me look that up." {
		t.Errorf("expected content 'Let me look that up.', got %s", resp.Content)
	}
	if resp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason 'tool_use', got %s", resp.StopReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].CallID != "call_123" {
		t.Errorf("expected tool call ID 'call_123', got %s", resp.ToolCalls[0].CallID)
	}
	if resp.ToolCalls[0].Name != "test_tool" {
		t.Errorf("expected tool call name 'test_tool', got %s", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["query"] != "hello" {
		t.Errorf("expected tool call argument query=hello, got %v", resp.ToolCalls[0].Arguments["query"])
	}
}

func TestAnthropicClient_ToolResultMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// 验证工具结果消息格式
		if len(reqBody.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(reqBody.Messages))
		}

		// 第一条消息应该是 tool_use (assistant)
		if reqBody.Messages[0].Role != "assistant" {
			t.Errorf("expected first message role assistant, got %s", reqBody.Messages[0].Role)
		}

		// 第二条消息应该是 tool_result (user)
		if reqBody.Messages[1].Role != "user" {
			t.Errorf("expected second message role user, got %s", reqBody.Messages[1].Role)
		}

		resp := anthropicResponse{
			ID:    "msg_test",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-5-sonnet-20241022",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Based on the search results..."},
			},
			StopReason: "end_turn",
			Usage: anthropicUsage{
				InputTokens:  25,
				OutputTokens: 15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key", server.URL)
	cfg := defaultModelConfig()
	messages := []LLMMessage{
		{
			Role: "assistant",
			ToolCalls: []ToolCallInfo{
				{CallID: "call_123", Name: "test_tool", Arguments: map[string]any{"query": "hello"}},
			},
		},
		{
			Role:       "user",
			ToolCallID: "call_123",
			ToolName:   "test_tool",
			ToolResult: "Search results for hello",
		},
	}

	resp, err := client.Chat(context.Background(), messages, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Based on the search results..." {
		t.Errorf("expected content 'Based on the search results...', got %s", resp.Content)
	}
}

func TestAnthropicClient_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		errResp := anthropicErrorResponse{
			Type: "error",
		}
		errResp.Error.Type = "invalid_request_error"
		errResp.Error.Message = "invalid model"
		json.NewEncoder(w).Encode(errResp)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key", server.URL)
	cfg := defaultModelConfig()
	messages := []LLMMessage{
		{Role: "user", Content: "Hello"},
	}

	_, err := client.Chat(context.Background(), messages, cfg, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAnthropicClient_MaxTokensStopReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			ID:    "msg_test",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-5-sonnet-20241022",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Truncated response..."},
			},
			StopReason: "max_tokens",
			Usage: anthropicUsage{
				InputTokens:  10,
				OutputTokens: 100,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAnthropicClient("test-api-key", server.URL)
	cfg := defaultModelConfig()
	messages := []LLMMessage{
		{Role: "user", Content: "Tell me a long story"},
	}

	resp, err := client.Chat(context.Background(), messages, cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StopReason != "max_tokens" {
		t.Errorf("expected stop_reason 'max_tokens', got %s", resp.StopReason)
	}
}

func TestAnthropicClient_EmptyContentMessage(t *testing.T) {
	client := NewAnthropicClient("test-api-key", "https://unused.example.com")
	cfg := defaultModelConfig()
	messages := []LLMMessage{
		{Role: "user", Content: ""},
	}

	_, err := client.Chat(context.Background(), messages, cfg, nil)
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

func TestAnthropicClient_InvalidRole(t *testing.T) {
	client := NewAnthropicClient("test-api-key", "https://unused.example.com")
	cfg := defaultModelConfig()
	messages := []LLMMessage{
		{Role: "invalid_role", Content: "test"},
	}

	_, err := client.Chat(context.Background(), messages, cfg, nil)
	if err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}
}
