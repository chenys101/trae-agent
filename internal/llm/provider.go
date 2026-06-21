package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Role 消息角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 单条消息。
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Request LLM 请求。
type Request struct {
	Model       string
	Messages    []Message
	System      string
	MaxTokens   int
	Temperature float64
	Tools       []ToolDef
}

// ToolCall 非流式工具调用。
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args"`
}

// ToolDef 工具定义。
type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// Usage token 用量统计。
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// StreamEvent 流式事件接口。
type StreamEvent interface{ isStreamEvent() }

// TextDelta 文本增量。
type TextDelta struct {
	Content string
}

func (TextDelta) isStreamEvent() {}

// ToolCallDelta 工具调用增量。
type ToolCallDelta struct {
	ID   string
	Name string
	// ArgsDelta 当前实现发送完整累积 args，非真增量。
	ArgsDelta string
}

func (ToolCallDelta) isStreamEvent() {}

// Done 流结束。
type Done struct {
	Usage      Usage
	StopReason string
}

func (Done) isStreamEvent() {}

// Error 流中错误。
type Error struct {
	Err error
}

func (Error) isStreamEvent() {}

// Provider LLM 供应商抽象。
type Provider interface {
	Name() string
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}

// NewProvider 按 provider 名构造实例。
func NewProvider(name, apiKey, baseURL, defaultModel string) (Provider, error) {
	switch name {
	case "anthropic":
		return NewAnthropic(apiKey, baseURL, defaultModel), nil
	case "openai":
		return NewOpenAI(apiKey, baseURL, defaultModel), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}

// parseSSELine 按 SSE 规范解析单行：以 ":" 分割 field/value，
// 去掉 value 开头的可选单空格。返回 field 名与 value。
// 注释行（以 ":" 开头）返回空 field。
func parseSSELine(line string) (field, value string) {
	if strings.HasPrefix(line, ":") {
		return "", ""
	}
	field, value, ok := strings.Cut(line, ":")
	if !ok {
		return field, ""
	}
	// SSE 规范：value 开头若有单个空格则去掉
	if strings.HasPrefix(value, " ") {
		value = value[1:]
	}
	return field, value
}
