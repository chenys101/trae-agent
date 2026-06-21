package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

// mockCompactProvider 返回固定摘要。
type mockCompactProvider struct {
	response string
}

func (m *mockCompactProvider) Name() string { return "mock" }
func (m *mockCompactProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.TextDelta{Content: m.response}
	ch <- llm.Done{}
	close(ch)
	return ch, nil
}

func TestCompactor_Compact_shortHistory(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(6))
	c := NewCompactor(&mockCompactProvider{response: "summary"}, cm, "test-model")

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
		{Role: llm.RoleAssistant, Content: "hello"},
	}
	result, err := c.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	// 消息数 <= keep，不压缩
	if len(result) != 2 {
		t.Errorf("short history should not compact, got %d messages", len(result))
	}
}

func TestCompactor_Compact_longHistory(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(2))
	c := NewCompactor(&mockCompactProvider{response: "this is the summary"}, cm, "test-model")

	// 5 条消息，keep 2，应压缩前 3 条为摘要
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "msg1"},
		{Role: llm.RoleAssistant, Content: "msg2"},
		{Role: llm.RoleUser, Content: "msg3"},
		{Role: llm.RoleAssistant, Content: "msg4"},
		{Role: llm.RoleUser, Content: "msg5"},
	}
	result, err := c.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	// 1 摘要 + 2 保留 = 3
	if len(result) != 3 {
		t.Fatalf("expected 3 messages after compact, got %d", len(result))
	}
	// 第一条应是摘要
	if !strings.Contains(result[0].Content, "summary") {
		t.Errorf("first message should be summary: %s", result[0].Content)
	}
	// 最后两条应是原 msg4, msg5
	if result[1].Content != "msg4" {
		t.Errorf("result[1] = %q, want msg4", result[1].Content)
	}
	if result[2].Content != "msg5" {
		t.Errorf("result[2] = %q, want msg5", result[2].Content)
	}
}

func TestCompactor_Compact_preservesToolCalls(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(2))
	c := NewCompactor(&mockCompactProvider{response: "summary"}, cm, "test-model")

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "old1"},
		{Role: llm.RoleAssistant, Content: "old2"},
		{
			Role:    llm.RoleAssistant,
			Content: "recent with tool",
			ToolCalls: []llm.ToolCall{
				{ID: "t1", Name: "read", Args: `{}`},
			},
		},
		{Role: llm.RoleUser, Content: "recent"},
	}
	result, err := c.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	// 保留的消息应包含 tool calls
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if len(result[1].ToolCalls) != 1 {
		t.Errorf("preserved message should keep tool calls, got %d", len(result[1].ToolCalls))
	}
}
