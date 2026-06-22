package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/cost"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/permission"
	"github.com/bytedance/trae-agent/internal/tool"
)

// TestE2E_toolLoop 验证 agent 能完成 read→edit→bash 完整工具闭环。
func TestE2E_toolLoop(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "hello.txt")
	os.WriteFile(testFile, []byte("Hello World"), 0o644)

	provider := &scriptedProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "call-1", Name: "read", ArgsDelta: `{"file_path":"` + testFile + `"}`},
				llm.Done{Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}, StopReason: "tool_use"},
			},
			{
				llm.ToolCallDelta{ID: "call-2", Name: "edit", ArgsDelta: `{"file_path":"` + testFile + `","old_string":"World","new_string":"Go"}`},
				llm.Done{Usage: llm.Usage{InputTokens: 15, OutputTokens: 5}, StopReason: "tool_use"},
			},
			{
				llm.ToolCallDelta{ID: "call-3", Name: "bash", ArgsDelta: `{"command":"cat ` + testFile + `"}`},
				llm.Done{Usage: llm.Usage{InputTokens: 20, OutputTokens: 5}, StopReason: "tool_use"},
			},
			{
				llm.TextDelta{Content: "Done! The file now says Hello Go."},
				llm.Done{Usage: llm.Usage{InputTokens: 25, OutputTokens: 10}, StopReason: "end_turn"},
			},
		},
	}

	registry := tool.NewRegistry(
		tool.NewRead(),
		tool.NewWrite(),
		tool.NewEdit(),
		tool.NewBash(10 * time.Second),
	)
	a := agent.New(provider, registry, agent.WithMaxSteps(10), agent.WithModel("test-model"))

	events := make(chan agent.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run(context.Background(), "read, edit, and cat the file", events)
	}()

	renderer := NewRenderer()
	var buf bytes.Buffer
	usage, renderErr := renderer.Render(events, &buf)
	if renderErr != nil {
		t.Fatalf("render: %v", renderErr)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("agent run: %v", err)
	}

	if !strings.Contains(buf.String(), "Hello Go") {
		t.Errorf("output missing 'Hello Go', got: %s", buf.String())
	}

	content, _ := os.ReadFile(testFile)
	if !strings.Contains(string(content), "Hello Go") {
		t.Errorf("file not edited, content: %s", content)
	}
	if strings.Contains(string(content), "World") {
		t.Errorf("file still contains 'World', content: %s", content)
	}

	if usage.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}
}

// TestE2E_jsonMode 验证 --json 模式输出结构化 JSON。
func TestE2E_jsonMode(t *testing.T) {
	provider := &scriptedProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.TextDelta{Content: "Hello from JSON mode"},
				llm.Done{Usage: llm.Usage{InputTokens: 5, OutputTokens: 3}, StopReason: "end_turn"},
			},
		},
	}

	registry := tool.NewRegistry()
	a := agent.New(provider, registry, agent.WithModel("gpt-4o"))

	events := make(chan agent.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run(context.Background(), "test prompt", events)
	}()

	var result jsonResult
	for ev := range events {
		switch e := ev.(type) {
		case agent.TextEvent:
			result.Text += e.Content
		case agent.DoneEvent:
			result.Usage = e.Usage
		}
	}
	if err := <-errCh; err != nil {
		t.Fatalf("agent run: %v", err)
	}

	if result.Text != "Hello from JSON mode" {
		t.Errorf("text = %q", result.Text)
	}
	if result.Usage.InputTokens != 5 {
		t.Errorf("input tokens = %d", result.Usage.InputTokens)
	}

	estCost, ok := cost.Estimate("gpt-4o", result.Usage)
	if !ok {
		t.Fatal("expected cost estimate for gpt-4o")
	}
	if estCost <= 0 {
		t.Errorf("expected positive cost, got %f", estCost)
	}
}

// TestE2E_permissionDeny 验证权限策略拦截危险命令。
func TestE2E_permissionDeny(t *testing.T) {
	provider := &scriptedProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "call-1", Name: "bash", ArgsDelta: `{"command":"rm -rf /"}`},
				llm.Done{Usage: llm.Usage{InputTokens: 5, OutputTokens: 5}, StopReason: "tool_use"},
			},
			{
				llm.TextDelta{Content: "OK"},
				llm.Done{Usage: llm.Usage{InputTokens: 10, OutputTokens: 2}, StopReason: "end_turn"},
			},
		},
	}

	registry := tool.NewRegistry(tool.NewBash(10 * time.Second))
	policy := permission.NewPolicy()
	a := agent.New(provider, registry,
		agent.WithMaxSteps(5),
		agent.WithModel("test"),
		agent.WithPolicy(policy),
	)

	events := make(chan agent.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run(context.Background(), "run dangerous command", events)
	}()

	var sawDeniedResult bool
	for ev := range events {
		if e, ok := ev.(agent.ToolResultEvent); ok {
			if e.Result.IsError && strings.Contains(e.Result.Content, "permission denied") {
				sawDeniedResult = true
			}
		}
	}
	<-errCh

	if !sawDeniedResult {
		t.Error("expected permission denied error in tool result")
	}
}

// scriptedProvider 按脚本序列响应多次 Stream 调用。
type scriptedProvider struct {
	scripts [][]llm.StreamEvent
	callIdx int
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	if p.callIdx >= len(p.scripts) {
		ch := make(chan llm.StreamEvent, 1)
		go func() {
			defer close(ch)
			ch <- llm.Done{Usage: llm.Usage{}, StopReason: "end_turn"}
		}()
		return ch, nil
	}
	script := p.scripts[p.callIdx]
	p.callIdx++
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
