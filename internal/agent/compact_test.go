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
	// 1 摘要 + 1 assistant 占位 + 2 保留 = 4
	if len(result) != 4 {
		t.Fatalf("expected 4 messages after compact, got %d", len(result))
	}
	// 第一条应是摘要
	if !strings.Contains(result[0].Content, "summary") {
		t.Errorf("first message should be summary: %s", result[0].Content)
	}
	// 第二条是 assistant 占位
	if result[1].Role != llm.RoleAssistant {
		t.Errorf("result[1] role = %v, want assistant", result[1].Role)
	}
	// 最后两条应是原 msg4, msg5
	if result[2].Content != "msg4" {
		t.Errorf("result[2] = %q, want msg4", result[2].Content)
	}
	if result[3].Content != "msg5" {
		t.Errorf("result[3] = %q, want msg5", result[3].Content)
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
	// 1 摘要 + 1 assistant 占位 + 2 保留 = 4
	// splitForCompact 会把带 ToolCalls 的 assistant 移到 recent 侧，
	// 避免切断 tool_call/tool_result 配对
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	// result[2] 应是带 tool calls 的 assistant（被移到 recent 侧）
	if len(result[2].ToolCalls) != 1 {
		t.Errorf("preserved message should keep tool calls, got %d", len(result[2].ToolCalls))
	}
}

// TestCompactor_Compact_preservesToolResultPairing 验证切分点恰好落在
// assistant(tool_calls) 与 tool(result) 之间时，不会破坏配对。
func TestCompactor_Compact_preservesToolResultPairing(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(2))
	c := NewCompactor(&mockCompactProvider{response: "summary"}, cm, "test-model")

	// keep=2，原始切分点在 index 3（recent = [tool_result, user]）
	// 若不调整，tool_result 会孤立（其 tool_call 在 toSummarize 侧被摘要掉）
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "q1"},
		{Role: llm.RoleAssistant, Content: "a1", ToolCalls: []llm.ToolCall{{ID: "t1", Name: "read", Args: `{}`}}},
		{Role: llm.RoleTool, Content: "result1", ToolCallID: "t1"},
		{Role: llm.RoleUser, Content: "q2"},
		{Role: llm.RoleAssistant, Content: "a2"},
		{Role: llm.RoleUser, Content: "q3"},
	}
	result, err := c.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	// 验证 recent 侧没有孤立的 tool 消息（即没有 RoleTool 出现在 recent 开头）
	// 摘要(0) + 占位(1) + recent...
	for i, m := range result {
		if m.Role == llm.RoleTool && i == 2 {
			t.Errorf("tool message should not be first in recent (would be orphaned): result[%d]=%+v", i, m)
		}
	}
	// 验证 recent 侧若有 tool 消息，其前必须有带 ToolCalls 的 assistant
	for i := 2; i < len(result); i++ {
		if result[i].Role == llm.RoleTool {
			if i == 2 || len(result[i-1].ToolCalls) == 0 {
				t.Errorf("orphaned tool message at result[%d]: no preceding assistant with ToolCalls", i)
			}
		}
	}
}
