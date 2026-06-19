// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bytedance/trae-agent-go/internal/tool"
)

const defaultAnthropicBaseURL = "https://api.anthropic.com/v1"

// AnthropicClient 是 Anthropic API 的客户端实现。
type AnthropicClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewAnthropicClient 创建一个新的 Anthropic 客户端。
// 如果 baseURL 为空，使用默认的 "https://api.anthropic.com/v1"。
func NewAnthropicClient(apiKey, baseURL string) *AnthropicClient {
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	return &AnthropicClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// --- Anthropic API 请求/响应类型 ---

// anthropicRequest 表示发送到 Anthropic /messages 端点的请求体。
type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    string              `json:"system,omitempty"`
	Messages  []anthropicMessage  `json:"messages"`
	Tools     []anthropicToolDef  `json:"tools,omitempty"`
}

// anthropicMessage 表示 Anthropic API 的消息格式。
type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string 或 []anthropicContentBlock
}

// anthropicContentBlock 表示 Anthropic API 消息中的内容块。
type anthropicContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     any            `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   any            `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

// anthropicToolDef 表示 Anthropic API 的工具定义。
type anthropicToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// anthropicResponse 表示 Anthropic /messages 端点的响应体。
type anthropicResponse struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Role       string                   `json:"role"`
	Content    []anthropicContentBlock  `json:"content"`
	Model      string                   `json:"model"`
	StopReason string                   `json:"stop_reason"`
	Usage      anthropicUsage           `json:"usage"`
}

// anthropicUsage 表示 Anthropic API 的用量信息。
type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// anthropicErrorResponse 表示 Anthropic API 的错误响应。
type anthropicErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Chat 发送消息到 Anthropic API 并返回响应。
func (c *AnthropicClient) Chat(ctx context.Context, req ChatRequest) (LLMResponse, error) {
	retryCfg := DefaultRetryConfig()
	retryCfg.MaxRetries = req.Config.MaxRetries

	return WithRetry(ctx, retryCfg, func() (LLMResponse, error) {
		return c.doChat(ctx, req)
	})
}

// doChat 执行实际的 Anthropic API 调用。
func (c *AnthropicClient) doChat(ctx context.Context, chatReq ChatRequest) (LLMResponse, error) {
	messages := chatReq.Messages
	config := chatReq.Config
	tools := chatReq.Tools
	var systemMsg string
	var apiMessages []anthropicMessage

	// 将 []LLMMessage 转换为 Anthropic API 格式
	for _, msg := range messages {
		if msg.Role == "system" {
			systemMsg = msg.Content
			continue
		}

		if msg.ToolResult != "" {
			// 工具结果转换为 tool_result 格式
			apiMessages = append(apiMessages, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{
					{
						Type:      "tool_result",
						ToolUseID: msg.ToolCallID,
						Content:   msg.ToolResult,
						IsError:   false,
					},
				},
			})
		} else if len(msg.ToolCalls) > 0 {
			// 工具调用转换为 tool_use 格式
			var blocks []anthropicContentBlock
			if msg.Content != "" {
				blocks = append(blocks, anthropicContentBlock{
					Type: "text",
					Text: msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.CallID,
					Name:  tc.Name,
					Input: tc.Arguments,
				})
			}
			apiMessages = append(apiMessages, anthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})
		} else {
			// 普通的 user/assistant 消息
			if msg.Content == "" {
				return LLMResponse{}, fmt.Errorf("message content is required for role %q", msg.Role)
			}
			role := msg.Role
			if role != "user" && role != "assistant" {
				return LLMResponse{}, fmt.Errorf("invalid message role: %s", msg.Role)
			}
			apiMessages = append(apiMessages, anthropicMessage{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	// 将 []tool.Tool 转换为 Anthropic tools 格式
	var toolDefs []anthropicToolDef
	for _, t := range tools {
		toolDefs = append(toolDefs, anthropicToolDef{
			Name:        t.GetName(),
			Description: t.GetDescription(),
			InputSchema: tool.GetInputSchema(t),
		})
	}

	// 构建请求体
	reqBody := anthropicRequest{
		Model:     config.Model,
		MaxTokens: config.MaxTokens,
		System:    systemMsg,
		Messages:  apiMessages,
		Tools:     toolDefs,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// 发送 POST 请求到 /messages
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp anthropicErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return LLMResponse{}, &HTTPError{StatusCode: resp.StatusCode, Message: errResp.Error.Message}
		}
		return LLMResponse{}, &HTTPError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	// 解析响应
	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return LLMResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// 提取 Content、ToolCalls、Usage、StopReason
	var content string
	var toolCalls []ToolCallInfo

	for _, block := range apiResp.Content {
		if block.Type == "text" {
			content += block.Text
		} else if block.Type == "tool_use" {
			arguments := make(map[string]any)
			if m, ok := block.Input.(map[string]any); ok {
				arguments = m
			}
			toolCalls = append(toolCalls, ToolCallInfo{
				CallID:    block.ID,
				Name:      block.Name,
				Arguments: arguments,
			})
		}
	}

	usage := LLMUsage{
		InputTokens:  apiResp.Usage.InputTokens,
		OutputTokens: apiResp.Usage.OutputTokens,
	}

	llmResp := LLMResponse{
		Content:    content,
		Usage:      usage,
		StopReason: apiResp.StopReason,
	}
	if len(toolCalls) > 0 {
		llmResp.ToolCalls = toolCalls
	}

	return llmResp, nil
}
