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

// openaiMockTool 实现 tool.Tool 接口用于测试。
type openaiMockTool struct {
	name        string
	description string
	parameters  []tool.ToolParameter
}

func (m *openaiMockTool) GetName() string                    { return m.name }
func (m *openaiMockTool) GetDescription() string              { return m.description }
func (m *openaiMockTool) GetParameters() []tool.ToolParameter { return m.parameters }
func (m *openaiMockTool) Execute(ctx context.Context, args map[string]any) (tool.ToolResult, error) {
	return tool.ToolResult{}, nil
}

func newOpenAIMockTool() *openaiMockTool {
	return &openaiMockTool{
		name:        "test_tool",
		description: "A test tool",
		parameters: []tool.ToolParameter{
			{Name: "query", Type: "string", Description: "The query", Required: true},
		},
	}
}

func openaiModelConfig() ModelConfig {
	return ModelConfig{
		Model:       "gpt-4",
		MaxTokens:   1024,
		MaxRetries:  0, // 测试时不重试
		Temperature: 0.7,
		APIKey:      "test-api-key",
	}
}

func TestOpenAIClient_NewClient(t *testing.T) {
	client := NewOpenAIClient("test-key", "")
	if client.baseURL != defaultOpenAIBaseURL {
		t.Errorf("expected default baseURL %s, got %s", defaultOpenAIBaseURL, client.baseURL)
	}
	if client.apiKey != "test-key" {
		t.Errorf("expected apiKey test-key, got %s", client.apiKey)
	}

	client2 := NewOpenAIClient("test-key", "https://custom.api.com/v1")
	if client2.baseURL != "https://custom.api.com/v1" {
		t.Errorf("expected custom baseURL, got %s", client2.baseURL)
	}
}

func TestOpenAIClient_NormalResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}

		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("expected Authorization Bearer header, got %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type header, got %s", r.Header.Get("Content-Type"))
		}

		var reqBody openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// 验证消息格式
		if len(reqBody.Messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" {
			t.Errorf("expected first message role system, got %s", reqBody.Messages[0].Role)
		}
		if reqBody.Messages[1].Role != "user" {
			t.Errorf("expected second message role user, got %s", reqBody.Messages[1].Role)
		}
		if reqBody.Messages[2].Role != "assistant" {
			t.Errorf("expected third message role assistant, got %s", reqBody.Messages[2].Role)
		}

		resp := openaiResponse{
			ID:     "chatcmpl-test",
			Object: "chat.completion",
			Model:  "gpt-4",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: "Hello! How can I help you?",
					},
					FinishReason: "stop",
				},
			},
			Usage: openaiUsage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenAIClient("test-api-key", server.URL)
	cfg := openaiModelConfig()
	messages := []LLMMessage{
		{Role: "system", Content: "You are a helpful assistant"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	resp, err := client.Chat(context.Background(), ChatRequest{Messages: messages, Config: cfg, Tools: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Hello! How can I help you?" {
		t.Errorf("expected content 'Hello! How can I help you?', got %s", resp.Content)
	}
	if resp.StopReason != "stop" {
		t.Errorf("expected stop_reason 'stop', got %s", resp.StopReason)
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

func TestOpenAIClient_ToolCallResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// 验证工具定义格式
		if len(reqBody.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(reqBody.Tools))
		}
		if reqBody.Tools[0].Type != "function" {
			t.Errorf("expected tool type 'function', got %s", reqBody.Tools[0].Type)
		}
		if reqBody.Tools[0].Function.Name != "test_tool" {
			t.Errorf("expected tool name 'test_tool', got %s", reqBody.Tools[0].Function.Name)
		}
		if reqBody.Tools[0].Function.Description != "A test tool" {
			t.Errorf("expected tool description 'A test tool', got %s", reqBody.Tools[0].Function.Description)
		}
		if !reqBody.Tools[0].Function.Strict {
			t.Error("expected tool strict to be true")
		}
		if reqBody.Tools[0].Function.Parameters == nil {
			t.Error("expected tool parameters to be set")
		}

		resp := openaiResponse{
			ID:     "chatcmpl-test",
			Object: "chat.completion",
			Model:  "gpt-4",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: nil,
						ToolCalls: []openaiToolCall{
							{
								ID:   "call_abc",
								Type: "function",
								Function: openaiFunctionCall{
									Name:      "test_tool",
									Arguments: `{"query":"hello"}`,
								},
							},
						},
					},
					FinishReason: "function_call",
				},
			},
			Usage: openaiUsage{
				PromptTokens:     15,
				CompletionTokens: 25,
				TotalTokens:      40,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenAIClient("test-api-key", server.URL)
	cfg := openaiModelConfig()
	messages := []LLMMessage{
		{Role: "user", Content: "Search for hello"},
	}

	tools := []tool.Tool{newOpenAIMockTool()}
	resp, err := client.Chat(context.Background(), ChatRequest{Messages: messages, Config: cfg, Tools: tools})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].CallID != "call_abc" {
		t.Errorf("expected tool call ID 'call_abc', got %s", resp.ToolCalls[0].CallID)
	}
	if resp.ToolCalls[0].Name != "test_tool" {
		t.Errorf("expected tool call name 'test_tool', got %s", resp.ToolCalls[0].Name)
	}
	if resp.ToolCalls[0].Arguments["query"] != "hello" {
		t.Errorf("expected tool call argument query=hello, got %v", resp.ToolCalls[0].Arguments["query"])
	}
	// StopReason 映射：function_call → tool_use
	if resp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason 'tool_use', got %s", resp.StopReason)
	}
}

func TestOpenAIClient_ToolResultMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		// 验证工具结果消息格式
		if len(reqBody.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(reqBody.Messages))
		}

		// 第一条消息应该是 assistant 带 tool_calls
		if reqBody.Messages[0].Role != "assistant" {
			t.Errorf("expected first message role assistant, got %s", reqBody.Messages[0].Role)
		}
		if len(reqBody.Messages[0].ToolCalls) != 1 {
			t.Errorf("expected 1 tool call in first message, got %d", len(reqBody.Messages[0].ToolCalls))
		}

		// 第二条消息应该是 role=tool
		if reqBody.Messages[1].Role != "tool" {
			t.Errorf("expected second message role tool, got %s", reqBody.Messages[1].Role)
		}
		if reqBody.Messages[1].ToolCallID != "call_abc" {
			t.Errorf("expected tool_call_id 'call_abc', got %s", reqBody.Messages[1].ToolCallID)
		}

		resp := openaiResponse{
			ID:     "chatcmpl-test",
			Object: "chat.completion",
			Model:  "gpt-4",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: "Based on the search results...",
					},
					FinishReason: "stop",
				},
			},
			Usage: openaiUsage{
				PromptTokens:     25,
				CompletionTokens: 15,
				TotalTokens:      40,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenAIClient("test-api-key", server.URL)
	cfg := openaiModelConfig()
	messages := []LLMMessage{
		{
			Role: "assistant",
			ToolCalls: []ToolCallInfo{
				{CallID: "call_abc", Name: "test_tool", Arguments: map[string]any{"query": "hello"}},
			},
		},
		{
			Role:       "user",
			ToolCallID: "call_abc",
			ToolName:   "test_tool",
			ToolResult: "Search results for hello",
		},
	}

	resp, err := client.Chat(context.Background(), ChatRequest{Messages: messages, Config: cfg, Tools: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Based on the search results..." {
		t.Errorf("expected content 'Based on the search results...', got %s", resp.Content)
	}
}

func TestOpenAIClient_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Header().Set("Content-Type", "application/json")
		errResp := openaiErrorResponse{}
		errResp.Error.Message = "Invalid API key"
		errResp.Error.Type = "invalid_request_error"
		json.NewEncoder(w).Encode(errResp)
	}))
	defer server.Close()

	client := NewOpenAIClient("bad-key", server.URL)
	cfg := openaiModelConfig()
	messages := []LLMMessage{
		{Role: "user", Content: "Hello"},
	}

	_, err := client.Chat(context.Background(), ChatRequest{Messages: messages, Config: cfg, Tools: nil})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOpenAIClient_LengthStopReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			ID:     "chatcmpl-test",
			Object: "chat.completion",
			Model:  "gpt-4",
			Choices: []openaiChoice{
				{
					Index: 0,
					Message: openaiMessage{
						Role:    "assistant",
						Content: "Truncated response...",
					},
					FinishReason: "length",
				},
			},
			Usage: openaiUsage{
				PromptTokens:     10,
				CompletionTokens: 100,
				TotalTokens:      110,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenAIClient("test-api-key", server.URL)
	cfg := openaiModelConfig()
	messages := []LLMMessage{
		{Role: "user", Content: "Tell me a long story"},
	}

	resp, err := client.Chat(context.Background(), ChatRequest{Messages: messages, Config: cfg, Tools: nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// StopReason 映射：length → max_tokens
	if resp.StopReason != "max_tokens" {
		t.Errorf("expected stop_reason 'max_tokens', got %s", resp.StopReason)
	}
}

func TestOpenAIClient_EmptyContentMessage(t *testing.T) {
	client := NewOpenAIClient("test-api-key", "https://unused.example.com")
	cfg := openaiModelConfig()
	messages := []LLMMessage{
		{Role: "user", Content: ""},
	}

	_, err := client.Chat(context.Background(), ChatRequest{Messages: messages, Config: cfg, Tools: nil})
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

func TestOpenAIClient_InvalidRole(t *testing.T) {
	client := NewOpenAIClient("test-api-key", "https://unused.example.com")
	cfg := openaiModelConfig()
	messages := []LLMMessage{
		{Role: "invalid_role", Content: "test"},
	}

	_, err := client.Chat(context.Background(), ChatRequest{Messages: messages, Config: cfg, Tools: nil})
	if err == nil {
		t.Fatal("expected error for invalid role, got nil")
	}
}

func TestMapOpenAIStopReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "stop"},
		{"function_call", "tool_use"},
		{"length", "max_tokens"},
		{"content_filter", "content_filter"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapOpenAIStopReason(tt.input)
			if got != tt.expected {
				t.Errorf("mapOpenAIStopReason(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
