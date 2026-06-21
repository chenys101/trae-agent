package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

// TestE2E_subagent_parallelExecution 验证主 agent 通过 Task 工具
// 并行派发两个子 agent，结果汇总回主 agent。
func TestE2E_subagent_parallelExecution(t *testing.T) {
	// 主 agent 的 provider：第一轮返回两个 Task 工具调用，第二轮返回汇总文本
	mainProvider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "tc1", Name: "task", ArgsDelta: `{"subagent_type":"search","description":"find TODOs","prompt":"find all TODO comments"}`},
				llm.ToolCallDelta{ID: "tc2", Name: "task", ArgsDelta: `{"subagent_type":"search","description":"find FIXMEs","prompt":"find all FIXME comments"}`},
				llm.Done{Usage: llm.Usage{InputTokens: 10, OutputTokens: 20}},
			},
			{
				llm.TextDelta{Content: "Found 3 TODOs and 2 FIXMEs"},
				llm.Done{Usage: llm.Usage{InputTokens: 50, OutputTokens: 10}},
			},
		},
	}

	// 子 agent 的 provider：两个子 agent 各返回不同文本
	// 注意：两个 Task 调用并行执行，但共享同一个 subProvider
	// mockProvider 的 scripts 按调用顺序消费，并行时顺序不确定
	// 所以让两个子 agent 返回相同结构的结果，主 agent 汇总时不在乎顺序
	subProvider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.TextDelta{Content: "found 3 TODOs"},
				llm.Done{},
			},
			{
				llm.TextDelta{Content: "found 2 FIXMEs"},
				llm.Done{},
			},
		},
	}

	baseTools := []tool.Tool{
		&mockTool{name: "read"},
		&mockTool{name: "glob"},
		&mockTool{name: "grep"},
	}
	baseRegistry := tool.NewRegistry(baseTools...)
	subagentRunner := NewSubagentRunner(subProvider, baseRegistry, "test-model")
	// 用 copy 避免 append 修改 baseTools 底层数组
	fullTools := make([]tool.Tool, len(baseTools))
	copy(fullTools, baseTools)
	fullRegistry := tool.NewRegistry(append(fullTools, tool.NewTask(subagentRunner))...)

	mainAgent := New(mainProvider, fullRegistry, WithModel("test-model"))

	events := make(chan Event, 64)
	var finalText string
	done := make(chan error, 1)
	go func() {
		done <- mainAgent.Run(context.Background(), "find TODOs and FIXMEs in parallel", events)
	}()

	for ev := range events {
		if te, ok := ev.(TextEvent); ok {
			finalText += te.Content
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("main agent failed: %v", err)
	}

	if !strings.Contains(finalText, "Found 3 TODOs and 2 FIXMEs") {
		t.Errorf("final text should contain summary: %q", finalText)
	}
}

// TestE2E_subagent_contextIsolation 验证子 agent 的消息历史
// 不污染主 agent 的消息历史。
func TestE2E_subagent_contextIsolation(t *testing.T) {
	mainProvider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "tc1", Name: "task", ArgsDelta: `{"subagent_type":"search","description":"test","prompt":"find something"}`},
				llm.Done{},
			},
			{
				llm.TextDelta{Content: "done"},
				llm.Done{},
			},
		},
	}

	// 子 agent 会调用 read 工具
	subProvider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "sub-tc1", Name: "read", ArgsDelta: `{"file_path":"/tmp/x"}`},
				llm.Done{},
			},
			{
				llm.TextDelta{Content: "subagent result"},
				llm.Done{},
			},
		},
	}

	baseTools := []tool.Tool{
		&mockTool{name: "read"},
	}
	baseRegistry := tool.NewRegistry(baseTools...)
	subagentRunner := NewSubagentRunner(subProvider, baseRegistry, "test-model")
	fullTools := make([]tool.Tool, len(baseTools))
	copy(fullTools, baseTools)
	fullRegistry := tool.NewRegistry(append(fullTools, tool.NewTask(subagentRunner))...)

	mainAgent := New(mainProvider, fullRegistry, WithModel("test-model"))

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "test"},
	}
	events := make(chan Event, 64)

	done := make(chan error, 1)
	go func() {
		done <- mainAgent.RunWithHistory(context.Background(), &messages, events)
	}()
	for range events {
	}
	<-done

	// 验证主 agent 的 messages 不含子 agent 的 read 工具调用
	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			if tc.Name == "read" {
				t.Errorf("main agent messages should not contain sub-agent's read tool call")
			}
		}
		// 子 agent 的 read 工具结果（"mock"）不应出现在主 agent 历史
		if m.Role == llm.RoleTool && strings.Contains(m.Content, "mock") {
			t.Errorf("main agent messages should not contain sub-agent's tool results: role=%v content=%q", m.Role, m.Content)
		}
	}
}

// TestE2E_subagent_failureDoesNotCrashMain 验证子 agent 失败时
// 主 agent 收到错误摘要，不崩溃。
func TestE2E_subagent_failureDoesNotCrashMain(t *testing.T) {
	mainProvider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "tc1", Name: "task", ArgsDelta: `{"subagent_type":"search","description":"fail","prompt":"fail this"}`},
				llm.Done{},
			},
			{
				llm.TextDelta{Content: "subagent failed, handled gracefully"},
				llm.Done{},
			},
		},
	}

	// 子 agent 的 provider 返回 error
	subProvider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.Error{Err: context.DeadlineExceeded},
			},
		},
	}

	baseTools := []tool.Tool{&mockTool{name: "read"}}
	baseRegistry := tool.NewRegistry(baseTools...)
	subagentRunner := NewSubagentRunner(subProvider, baseRegistry, "test-model")
	fullTools := make([]tool.Tool, len(baseTools))
	copy(fullTools, baseTools)
	fullRegistry := tool.NewRegistry(append(fullTools, tool.NewTask(subagentRunner))...)

	mainAgent := New(mainProvider, fullRegistry, WithModel("test-model"))

	events := make(chan Event, 64)
	done := make(chan error, 1)
	go func() {
		done <- mainAgent.Run(context.Background(), "test", events)
	}()

	var finalText string
	for ev := range events {
		if te, ok := ev.(TextEvent); ok {
			finalText += te.Content
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("main agent should not fail when subagent fails: %v", err)
	}
	if !strings.Contains(finalText, "handled gracefully") {
		t.Errorf("main agent should continue after subagent failure: %q", finalText)
	}
}
