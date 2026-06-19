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

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// OpenAIClient 是 OpenAI API 的客户端实现。
type OpenAIClient struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAIClient 创建一个新的 OpenAI 客户端。
// 如果 baseURL 为空，使用默认的 "https://api.openai.com/v1"。
func NewOpenAIClient(apiKey, baseURL string) *OpenAIClient {
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	return &OpenAIClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

// --- OpenAI API 请求/响应类型 ---

// openaiRequest 表示发送到 OpenAI /chat/completions 端点的请求体。
type openaiRequest struct {
	Model       string            `json:"model"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Messages    []openaiMessage   `json:"messages"`
	Tools       []openaiToolDef   `json:"tools,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
}

// openaiMessage 表示 OpenAI API 的消息格式。
type openaiMessage struct {
	Role       string              `json:"role"`
	Content    any                 `json:"content"` // string 或 nil
	ToolCalls  []openaiToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
}

// openaiToolCall 表示 OpenAI API 响应中的工具调用。
type openaiToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function openaiFunctionCall   `json:"function"`
}

// openaiFunctionCall 表示 OpenAI API 的函数调用。
type openaiFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openaiToolDef 表示 OpenAI API 的工具定义。
type openaiToolDef struct {
	Type     string              `json:"type"`
	Function openaiFunctionDef   `json:"function"`
}

// openaiFunctionDef 表示 OpenAI API 的函数定义。
type openaiFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}

// openaiResponse 表示 OpenAI /chat/completions 端点的响应体。
type openaiResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Model   string         `json:"model"`
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

// openaiChoice 表示 OpenAI API 响应中的选项。
type openaiChoice struct {
	Index        int            `json:"index"`
	Message      openaiMessage  `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

// openaiUsage 表示 OpenAI API 的用量信息。
type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openaiErrorResponse 表示 OpenAI API 的错误响应。
type openaiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// Chat 发送消息到 OpenAI API 并返回响应。
func (c *OpenAIClient) Chat(ctx context.Context, req ChatRequest) (LLMResponse, error) {
	retryCfg := DefaultRetryConfig()
	retryCfg.MaxRetries = req.Config.MaxRetries

	return WithRetry(ctx, retryCfg, func() (LLMResponse, error) {
		return c.doChat(ctx, req)
	})
}

// doChat 执行实际的 OpenAI API 调用。
func (c *OpenAIClient) doChat(ctx context.Context, chatReq ChatRequest) (LLMResponse, error) {
	messages := chatReq.Messages
	config := chatReq.Config
	tools := chatReq.Tools
	var apiMessages []openaiMessage

	// 将 []LLMMessage 转换为 OpenAI API 格式
	for _, msg := range messages {
		if msg.ToolResult != "" {
			// 工具结果转换为 role=tool 格式
			apiMessages = append(apiMessages, openaiMessage{
				Role:       "tool",
				Content:    msg.ToolResult,
				ToolCallID: msg.ToolCallID,
			})
		} else if len(msg.ToolCalls) > 0 {
			// 工具调用转换为 function 格式
			var oaiToolCalls []openaiToolCall
			for _, tc := range msg.ToolCalls {
				argsJSON := tool.MarshalArguments(tc.Arguments)
				oaiToolCalls = append(oaiToolCalls, openaiToolCall{
					ID:   tc.CallID,
					Type: "function",
					Function: openaiFunctionCall{
						Name:      tc.Name,
						Arguments: argsJSON,
					},
				})
			}
			apiMessages = append(apiMessages, openaiMessage{
				Role:      "assistant",
				Content:   msg.Content,
				ToolCalls: oaiToolCalls,
			})
		} else {
			// 普通的 system/user/assistant 消息
			if msg.Content == "" {
				return LLMResponse{}, fmt.Errorf("message content is required for role %q", msg.Role)
			}
			role := msg.Role
			if role != "system" && role != "user" && role != "assistant" {
				return LLMResponse{}, fmt.Errorf("invalid message role: %s", msg.Role)
			}
			apiMessages = append(apiMessages, openaiMessage{
				Role:    role,
				Content: msg.Content,
			})
		}
	}

	// 将 []tool.Tool 转换为 OpenAI functions 格式
	var toolDefs []openaiToolDef
	for _, t := range tools {
		toolDefs = append(toolDefs, openaiToolDef{
			Type: "function",
			Function: openaiFunctionDef{
				Name:        t.GetName(),
				Description: t.GetDescription(),
				Parameters:  tool.GetInputSchema(t),
				Strict:      true,
			},
		})
	}

	// 构建请求体
	reqBody := openaiRequest{
		Model:     config.Model,
		MaxTokens: config.MaxTokens,
		Messages:  apiMessages,
		Tools:     toolDefs,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// 发送 POST 请求到 /chat/completions
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return LLMResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

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
		var errResp openaiErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return LLMResponse{}, &HTTPError{StatusCode: resp.StatusCode, Message: errResp.Error.Message}
		}
		return LLMResponse{}, &HTTPError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	// 解析响应
	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return LLMResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return LLMResponse{}, fmt.Errorf("openai API returned no choices")
	}

	choice := apiResp.Choices[0]

	// 提取 Content、ToolCalls
	content, _ := choice.Message.Content.(string)

	var toolCalls []ToolCallInfo
	for _, tc := range choice.Message.ToolCalls {
		arguments := make(map[string]any)
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &arguments); err != nil {
				return LLMResponse{}, fmt.Errorf("failed to parse tool call arguments: %w", err)
			}
		}
		toolCalls = append(toolCalls, ToolCallInfo{
			CallID:    tc.ID,
			Name:      tc.Function.Name,
			Arguments: arguments,
		})
	}

	usage := LLMUsage{
		InputTokens:  apiResp.Usage.PromptTokens,
		OutputTokens: apiResp.Usage.CompletionTokens,
	}

	// StopReason 映射：stop→stop, function_call→tool_use, length→max_tokens
	stopReason := mapOpenAIStopReason(choice.FinishReason)

	llmResp := LLMResponse{
		Content:    content,
		Usage:      usage,
		StopReason: stopReason,
	}
	if len(toolCalls) > 0 {
		llmResp.ToolCalls = toolCalls
	}

	return llmResp, nil
}

// mapOpenAIStopReason 将 OpenAI 的 finish_reason 映射为标准化的 stop_reason。
func mapOpenAIStopReason(reason string) string {
	switch reason {
	case "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}
