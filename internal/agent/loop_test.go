package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

// mockProvider 按预设脚本返回事件流。
type mockProvider struct {
	scripts [][]llm.StreamEvent // 每次调用返回一个脚本
	calls   int
	block   bool // 如果为 true，Stream 阻塞直到 ctx 取消，返回 ctx.Err()
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	// 阻塞模式：用于测试 ctx 取消时 agent 的退出行为
	if m.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	script := m.scripts[m.calls]
	m.calls++
	ch := make(chan llm.StreamEvent, len(script))
	go func() {
		defer close(ch)
		for _, e := range script {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch, nil
}

func TestAgent_singleTextResponse(t *testing.T) {
	provider := &mockProvider{scripts: [][]llm.StreamEvent{
		{
			llm.TextDelta{Content: "hello"},
			llm.Done{Usage: llm.Usage{InputTokens: 5, OutputTokens: 3}, StopReason: "end_turn"},
		},
	}}
	registry := tool.NewRegistry()
	a := New(provider, registry, WithMaxSteps(5))

	events := make(chan Event, 10)
	go func() {
		a.Run(context.Background(), "hi", events)
	}()

	var buf bytes.Buffer
	var usage llm.Usage
	for ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			buf.WriteString(e.Content)
		case DoneEvent:
			usage = e.Usage
		}
	}
	if buf.String() != "hello" {
		t.Errorf("output = %q, want hello", buf.String())
	}
	if usage.InputTokens != 5 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestAgent_toolCallLoop(t *testing.T) {
	// step1: 调 echo 工具
	// step2: 收到工具结果后，输出文本结束
	provider := &mockProvider{scripts: [][]llm.StreamEvent{
		{
			llm.ToolCallDelta{ID: "t1", Name: "echo", ArgsDelta: `{"msg":"hi"}`},
			llm.Done{StopReason: "tool_use"},
		},
		{
			llm.TextDelta{Content: "done"},
			llm.Done{StopReason: "end_turn"},
		},
	}}

	echo := testEchoTool{}
	registry := tool.NewRegistry(echo)
	a := New(provider, registry, WithMaxSteps(5))

	events := make(chan Event, 20)
	go func() {
		a.Run(context.Background(), "call echo", events)
	}()

	var buf bytes.Buffer
	for ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			buf.WriteString(e.Content)
		case ToolCallEvent:
			buf.WriteString(e.Name)
		}
	}

	out := buf.String()
	if !strings.Contains(out, "echo") {
		t.Errorf("missing tool call: %s", out)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("missing final text: %s", out)
	}
}

// testEchoTool 测试用 echo 工具
type testEchoTool struct{}

func (testEchoTool) Name() string            { return "echo" }
func (testEchoTool) Description() string     { return "echo" }
func (testEchoTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (testEchoTool) Run(ctx context.Context, args json.RawMessage) tool.Result {
	var v struct{ Msg string `json:"msg"` }
	json.Unmarshal(args, &v)
	return tool.Result{Content: v.Msg}
}

// TestAgent_maxStepsExceeded 验证连续工具调用达到 maxSteps 时返回 "max steps (N) exceeded" error。
// mockProvider 每次都返回 tool_call，maxSteps=2，循环执行 2 步后退出并返回 error。
func TestAgent_maxStepsExceeded(t *testing.T) {
	toolScript := []llm.StreamEvent{
		llm.ToolCallDelta{ID: "t1", Name: "echo", ArgsDelta: `{"msg":"hi"}`},
		llm.Done{StopReason: "tool_use"},
	}
	// 多准备几个脚本，避免 provider 被多调用时越界
	provider := &mockProvider{scripts: [][]llm.StreamEvent{
		toolScript, toolScript, toolScript,
	}}
	echo := testEchoTool{}
	registry := tool.NewRegistry(echo)
	a := New(provider, registry, WithMaxSteps(2))

	events := make(chan Event, 20)
	go func() { for range events {} }() // 排空 events，避免发送阻塞

	err := a.Run(context.Background(), "call echo", events)
	if err == nil {
		t.Fatal("expected max steps exceeded error, got nil")
	}
	if !strings.Contains(err.Error(), "max steps (2) exceeded") {
		t.Errorf("error should contain 'max steps (2) exceeded', got %v", err)
	}
	// maxSteps=2，provider 应被调用 2 次（step 0 和 step 1），第 3 步不会执行
	if provider.calls != 2 {
		t.Errorf("provider calls = %d, want 2", provider.calls)
	}
}

// TestAgent_contextCancel 验证 ctx 取消时 agent 正确退出。
// mockProvider 在 Stream 时阻塞，ctx cancel 后 agent 返回包装了 ctx.Err() 的 error。
func TestAgent_contextCancel(t *testing.T) {
	provider := &mockProvider{block: true}
	registry := tool.NewRegistry()
	a := New(provider, registry, WithMaxSteps(5))

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan Event, 10)
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run(ctx, "hi", events)
	}()

	cancel()
	select {
	case err := <-errCh:
		// Stream 返回 ctx.Err()，被 RunWithHistory 包装为 "step N stream: %w"
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected error wrapping context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not return after ctx cancel")
	}
}

// TestAgent_toolResultAppendedToHistory 验证 tool result 正确回灌到 messages。
// 第一轮返回 tool_call，第二轮返回 text，验证 messages 包含 assistant(tool_calls) + tool(result) 配对。
func TestAgent_toolResultAppendedToHistory(t *testing.T) {
	provider := &mockProvider{scripts: [][]llm.StreamEvent{
		{
			llm.ToolCallDelta{ID: "t1", Name: "echo", ArgsDelta: `{"msg":"hi"}`},
			llm.Done{StopReason: "tool_use"},
		},
		{
			llm.TextDelta{Content: "done"},
			llm.Done{StopReason: "end_turn"},
		},
	}}
	echo := testEchoTool{}
	registry := tool.NewRegistry(echo)
	a := New(provider, registry, WithMaxSteps(5))

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "call echo"},
	}
	events := make(chan Event, 20)
	go func() { for range events {} }() // 排空 events

	if err := a.RunWithHistory(context.Background(), &messages, events); err != nil {
		t.Fatalf("RunWithHistory returned error: %v", err)
	}

	// 预期 messages: [user, assistant(tool_calls), tool(result), assistant(text)]
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(messages), messages)
	}
	// messages[1] 应是带 tool_calls 的 assistant
	if messages[1].Role != llm.RoleAssistant || len(messages[1].ToolCalls) != 1 {
		t.Errorf("messages[1] should be assistant with 1 tool call, got %+v", messages[1])
	}
	if messages[1].ToolCalls[0].ID != "t1" || messages[1].ToolCalls[0].Name != "echo" {
		t.Errorf("messages[1].ToolCalls[0] = %+v, want ID=t1 Name=echo", messages[1].ToolCalls[0])
	}
	// messages[2] 应是 tool result，ToolCallID 关联 t1
	if messages[2].Role != llm.RoleTool || messages[2].ToolCallID != "t1" {
		t.Errorf("messages[2] should be tool result with ToolCallID t1, got %+v", messages[2])
	}
	if messages[2].Content != "hi" {
		t.Errorf("messages[2] content = %q, want hi", messages[2].Content)
	}
	// messages[3] 应是最终 assistant 文本
	if messages[3].Role != llm.RoleAssistant || messages[3].Content != "done" {
		t.Errorf("messages[3] should be assistant 'done', got %+v", messages[3])
	}
}

// TestAgent_streamErrorWrapped 验证 llm.Error 事件被包装为 "step N stream error: ..."。
func TestAgent_streamErrorWrapped(t *testing.T) {
	provider := &mockProvider{scripts: [][]llm.StreamEvent{
		{
			llm.Error{Err: errors.New("boom")},
		},
	}}
	registry := tool.NewRegistry()
	a := New(provider, registry, WithMaxSteps(5))

	events := make(chan Event, 10)
	go func() { for range events {} }() // 排空 events

	err := a.Run(context.Background(), "hi", events)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "step 0 stream error:") {
		t.Errorf("error should contain 'step 0 stream error:', got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should wrap 'boom', got %v", err)
	}
}

// TestAgent_toolCallMultiDeltaAccumulated 验证多个 ToolCallDelta（同 ID）被累积为单个 ToolCall。
// mockProvider 分两次发送同 ID 的 delta，验证 toolCalls 只有 1 个且 Args 是累积值。
func TestAgent_toolCallMultiDeltaAccumulated(t *testing.T) {
	provider := &mockProvider{scripts: [][]llm.StreamEvent{
		{
			// 同 ID 的两个 delta，ArgsDelta 分片发送
			llm.ToolCallDelta{ID: "t1", Name: "echo", ArgsDelta: `{"msg":"`},
			llm.ToolCallDelta{ID: "t1", ArgsDelta: `hi"}`},
			llm.Done{StopReason: "tool_use"},
		},
		{
			llm.TextDelta{Content: "done"},
			llm.Done{StopReason: "end_turn"},
		},
	}}
	echo := testEchoTool{}
	registry := tool.NewRegistry(echo)
	a := New(provider, registry, WithMaxSteps(5))

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "call echo"},
	}
	events := make(chan Event, 20)
	go func() { for range events {} }() // 排空 events

	if err := a.RunWithHistory(context.Background(), &messages, events); err != nil {
		t.Fatalf("RunWithHistory returned error: %v", err)
	}

	// messages[1] 应是 assistant，只有 1 个累积的 ToolCall，Args 为累积值
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}
	assistantMsg := messages[1]
	if assistantMsg.Role != llm.RoleAssistant {
		t.Fatalf("messages[1] role = %v, want assistant", assistantMsg.Role)
	}
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("expected 1 accumulated tool call, got %d: %+v", len(assistantMsg.ToolCalls), assistantMsg.ToolCalls)
	}
	tc := assistantMsg.ToolCalls[0]
	if tc.ID != "t1" || tc.Name != "echo" {
		t.Errorf("tool call ID/Name = %q/%q, want t1/echo", tc.ID, tc.Name)
	}
	wantArgs := `{"msg":"hi"}`
	if tc.Args != wantArgs {
		t.Errorf("accumulated args = %q, want %q", tc.Args, wantArgs)
	}
}
