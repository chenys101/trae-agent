package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

// mockProvider 按预设脚本返回事件流。
type mockProvider struct {
	scripts [][]llm.StreamEvent // 每次调用返回一个脚本
	calls   int
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
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
	usage, err := RenderEvents(context.Background(), events, &buf)
	if err != nil {
		t.Fatal(err)
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
	RenderEvents(context.Background(), events, &buf)

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
