package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// Role 消息角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 单条消息。M1 只用 Content 文本，ToolCalls 留 M2。
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
	ID   string
	Name string
	Args string
}

// ToolDef 工具定义。
type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// Usage token 用量统计。
type Usage struct {
	InputTokens  int
	OutputTokens int
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
	ID        string
	Name      string
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
