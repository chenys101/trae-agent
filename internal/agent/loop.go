package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

// Agent 多步 think-act 循环。
type Agent struct {
	provider   llm.Provider
	dispatcher *tool.Dispatcher
	tools      []llm.ToolDef
	maxSteps   int
}

type Option func(*Agent)

func WithMaxSteps(n int) Option {
	return func(a *Agent) { a.maxSteps = n }
}

func New(provider llm.Provider, registry *tool.Registry, opts ...Option) *Agent {
	a := &Agent{
		provider:   provider,
		dispatcher: tool.NewDispatcher(registry),
		maxSteps:   20,
	}
	for _, o := range opts {
		o(a)
	}
	// 构造 tool defs
	for _, t := range registry.List() {
		a.tools = append(a.tools, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Schema(),
		})
	}
	return a
}

// Event agent 循环产生的事件，供 UI 渲染。
type Event interface{ isAgentEvent() }

type TextEvent struct{ Content string }
type ToolCallEvent struct {
	Name string
	Args string
}
type ToolResultEvent struct {
	Name   string
	Result tool.Result
}
type DoneEvent struct{ Usage llm.Usage }

func (TextEvent) isAgentEvent()       {}
func (ToolCallEvent) isAgentEvent()   {}
func (ToolResultEvent) isAgentEvent() {}
func (DoneEvent) isAgentEvent()       {}

// Run 执行 agent 循环，处理用户输入，通过 events channel 推送事件。
func (a *Agent) Run(ctx context.Context, userInput string, events chan<- Event) error {
	defer close(events)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: userInput},
	}

	for step := 0; step < a.maxSteps; step++ {
		req := llm.Request{
			Model:    "",
			System:   SystemPrompt,
			Messages: messages,
			Tools:    a.tools,
		}

		ch, err := a.provider.Stream(ctx, req)
		if err != nil {
			return fmt.Errorf("step %d stream: %w", step, err)
		}

		var textBuf string
		var toolCalls []llm.ToolCall
		var usage llm.Usage

		for ev := range ch {
			switch e := ev.(type) {
			case llm.TextDelta:
				textBuf += e.Content
				select {
				case events <- TextEvent{Content: e.Content}:
				case <-ctx.Done():
					return ctx.Err()
				}
			case llm.ToolCallDelta:
				tc := llm.ToolCall{
					ID:   e.ID,
					Name: e.Name,
					Args: e.ArgsDelta,
				}
				toolCalls = append(toolCalls, tc)
				select {
				case events <- ToolCallEvent{Name: tc.Name, Args: tc.Args}:
				case <-ctx.Done():
					return ctx.Err()
				}
			case llm.Done:
				usage = e.Usage
			case llm.Error:
				return e.Err
			}
		}

		// assistant 消息回灌
		assistantMsg := llm.Message{Role: llm.RoleAssistant, Content: textBuf, ToolCalls: toolCalls}
		messages = append(messages, assistantMsg)

		// 没有工具调用，循环结束
		if len(toolCalls) == 0 {
			select {
			case events <- DoneEvent{Usage: usage}:
			case <-ctx.Done():
			}
			return nil
		}

		// 执行工具
		calls := make([]tool.Call, len(toolCalls))
		for i, tc := range toolCalls {
			calls[i] = tool.Call{Name: tc.Name, Args: []byte(tc.Args)}
		}
		results := a.dispatcher.Dispatch(ctx, calls)
		for i, r := range results {
			select {
			case events <- ToolResultEvent{Name: r.Name, Result: r.Result}:
			case <-ctx.Done():
				return ctx.Err()
			}
			// 工具结果回灌：用 ToolCallID 关联，provider 层负责序列化为
			// anthropic tool_result block 或 openai role=tool 消息
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    r.Result.Content,
				ToolCallID: toolCalls[i].ID,
			})
		}
	}

	select {
	case events <- DoneEvent{Usage: llm.Usage{}}:
	case <-ctx.Done():
	}
	return fmt.Errorf("max steps (%d) exceeded", a.maxSteps)
}

// RenderEvents 消费 agent 事件，渲染到 out（简单文本版）。
func RenderEvents(ctx context.Context, events <-chan Event, out io.Writer) (llm.Usage, error) {
	var usage llm.Usage
	for ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			fmt.Fprint(out, e.Content)
		case ToolCallEvent:
			fmt.Fprintf(out, "\n[tool: %s %s]\n", e.Name, e.Args)
		case ToolResultEvent:
			fmt.Fprintf(out, "[result: %s]\n%s\n", e.Name, e.Result.Content)
		case DoneEvent:
			usage = e.Usage
		}
	}
	return usage, nil
}
