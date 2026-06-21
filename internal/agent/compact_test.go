package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/trae-agent/internal/llm"
)

// mockCompactProvider 返回固定摘要。
type mockCompactProvider struct {
	response string
	failWith error // 如果非 nil，发送 llm.Error 事件而非文本
	block    bool  // 如果为 true，Stream 阻塞直到 ctx 取消，返回 ctx.Err()
}

func (m *mockCompactProvider) Name() string { return "mock" }
func (m *mockCompactProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	// 阻塞模式：用于测试 ctx 取消时 Compact 的行为
	if m.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ch := make(chan llm.StreamEvent, 2)
	if m.failWith != nil {
		ch <- llm.Error{Err: m.failWith}
	} else {
		ch <- llm.TextDelta{Content: m.response}
	}
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

// TestCompactor_Compact_summarizeFailureFallback 验证 summarize 失败时降级为截断策略。
// mockCompactProvider 返回 llm.Error，验证 Compact 返回非 nil（降级结果）且包含 recent 消息。
func TestCompactor_Compact_summarizeFailureFallback(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(2))
	c := NewCompactor(&mockCompactProvider{failWith: errors.New("summarize failed")}, cm, "test-model")

	// 5 条消息，keep=2，前 3 条需摘要，但 summarize 失败
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "old1"},
		{Role: llm.RoleAssistant, Content: "old2"},
		{Role: llm.RoleUser, Content: "old3"},
		{Role: llm.RoleAssistant, Content: "recent1"},
		{Role: llm.RoleUser, Content: "recent2"},
	}
	result, err := c.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatalf("Compact should not return error on summarize failure (fallback), got %v", err)
	}
	if result == nil {
		t.Fatal("fallback result should not be nil")
	}
	// 降级结果：truncated user + assistant "Understood." + recent(2) = 4
	if len(result) != 4 {
		t.Fatalf("expected 4 messages (2 fallback + 2 recent), got %d", len(result))
	}
	// 第一条应是截断提示
	if !strings.Contains(result[0].Content, "truncated") {
		t.Errorf("first message should mention truncation, got %q", result[0].Content)
	}
	// recent 消息应保留在结果末尾
	if result[len(result)-1].Content != "recent2" {
		t.Errorf("last message should be recent2, got %q", result[len(result)-1].Content)
	}
	if result[len(result)-2].Content != "recent1" {
		t.Errorf("second to last should be recent1, got %q", result[len(result)-2].Content)
	}
}

// TestCompactor_Compact_contextCancel 验证 ctx 取消时 Compact 不挂起并返回降级结果。
// mockCompactProvider 阻塞，ctx cancel 后 summarize 返回 ctx.Err()，
// Compact 的降级策略会吞掉该 error 并返回截断结果（含 recent 消息）。
func TestCompactor_Compact_contextCancel(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(2))
	c := NewCompactor(&mockCompactProvider{block: true}, cm, "test-model")

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "old1"},
		{Role: llm.RoleAssistant, Content: "old2"},
		{Role: llm.RoleUser, Content: "old3"},
		{Role: llm.RoleAssistant, Content: "recent1"},
		{Role: llm.RoleUser, Content: "recent2"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	type compactResult struct {
		msgs []llm.Message
		err  error
	}
	resultCh := make(chan compactResult, 1)
	go func() {
		r, e := c.Compact(ctx, msgs)
		resultCh <- compactResult{msgs: r, err: e}
	}()

	cancel()
	select {
	case got := <-resultCh:
		// summarize 返回 ctx.Err()，但 Compact 降级策略吞掉该 error，
		// 返回截断结果（nil error）。这里验证 Compact 不挂起、返回非 nil 降级结果且包含 recent 消息。
		if got.err != nil {
			t.Errorf("Compact returned error %v (fallback swallows ctx error)", got.err)
		}
		if got.msgs == nil {
			t.Fatal("fallback result should not be nil")
		}
		// 降级结果：truncated user + assistant "Understood." + recent(2) = 4
		if len(got.msgs) != 4 {
			t.Fatalf("expected 4 messages (2 fallback + 2 recent), got %d", len(got.msgs))
		}
		// recent 消息应保留在末尾
		if got.msgs[len(got.msgs)-1].Content != "recent2" {
			t.Errorf("last message should be recent2, got %q", got.msgs[len(got.msgs)-1].Content)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Compact did not return after ctx cancel")
	}
}
