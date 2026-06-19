// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

// Package llm 定义了 LLM 客户端的核心接口和数据结构。
package llm

import (
	"context"
	"fmt"

	"github.com/bytedance/trae-agent-go/internal/tool"
)

// ChatRequest encapsulates all parameters for a chat request.
type ChatRequest struct {
	Messages []LLMMessage
	Config   ModelConfig
	Tools    []tool.Tool
}

// LLMClient 是 LLM 客户端的核心接口。
type LLMClient interface {
	Chat(ctx context.Context, req ChatRequest) (LLMResponse, error)
}

// LLMMessage 表示发送给 LLM 的消息。
type LLMMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []ToolCallInfo `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	ToolResult string         `json:"tool_result,omitempty"`
	Images     []ImageContent `json:"images,omitempty"`
}

// ToolCallInfo 记录 LLM 发起的工具调用信息。
type ToolCallInfo struct {
	Name      string         `json:"name"`
	CallID    string         `json:"call_id"`
	Arguments map[string]any `json:"arguments"`
}

// ImageContent 表示多模态图片内容。
type ImageContent struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// LLMResponse 表示 LLM 的响应。
type LLMResponse struct {
	Content    string         `json:"content"`
	ToolCalls  []ToolCallInfo `json:"tool_calls,omitempty"`
	Usage      LLMUsage       `json:"usage"`
	StopReason string         `json:"stop_reason"`
}

// LLMUsage 记录 LLM 的 token 使用量。
type LLMUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ModelConfig 定义 LLM 调用的模型配置。
type ModelConfig struct {
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	MaxTokens        int     `json:"max_tokens"`
	Temperature      float64 `json:"temperature"`
	TopP             float64 `json:"top_p"`
	TopK             int     `json:"top_k"`
	MaxRetries       int     `json:"max_retries"`
	ParallelToolCalls bool   `json:"parallel_tool_calls"`
	APIKey           string  `json:"api_key"`
	BaseURL          string  `json:"base_url"`
}

// APIError represents an error from an LLM API call.
type APIError struct {
	Provider   string // "anthropic" or "openai"
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s API error (status %d): %s", e.Provider, e.StatusCode, e.Message)
}

// NewClient 根据 provider 返回对应的 LLM 客户端实例。
func NewClient(provider string, apiKey string, baseURL string) (LLMClient, error) {
	switch provider {
	case "anthropic":
		return NewAnthropicClient(apiKey, baseURL), nil
	case "openai":
		return NewOpenAIClient(apiKey, baseURL), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}
