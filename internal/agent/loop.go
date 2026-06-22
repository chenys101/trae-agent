package agent

import (
	"context"
	"fmt"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/permission"
	"github.com/bytedance/trae-agent/internal/tool"
)

// Agent 多步 think-act 循环。
type Agent struct {
	provider     llm.Provider
	dispatcher   *tool.Dispatcher
	tools        []llm.ToolDef
	maxSteps     int
	model        string
	systemPrompt string // 系统提示词，默认用 prompt.go 的 SystemPrompt
	policy       permission.Policy
	asker        permission.Asker
}

type Option func(*Agent)

func WithMaxSteps(n int) Option {
	return func(a *Agent) { a.maxSteps = n }
}

func WithModel(m string) Option {
	return func(a *Agent) { a.model = m }
}

// WithSystemPrompt 设置 agent 的系统提示词，覆盖默认的 SystemPrompt。
func WithSystemPrompt(p string) Option {
	return func(a *Agent) { a.systemPrompt = p }
}

// WithPolicy 设置权限策略，启用权限检查。
func WithPolicy(p permission.Policy) Option {
	return func(a *Agent) { a.policy = p }
}

// WithAsker 设置交互式 asker，处理 ActionAsk 决策。
func WithAsker(asker permission.Asker) Option {
	return func(a *Agent) { a.asker = asker }
}

func New(provider llm.Provider, registry *tool.Registry, opts ...Option) *Agent {
	a := &Agent{
		provider:     provider,
		maxSteps:     20,
		systemPrompt: SystemPrompt, // 默认用 prompt.go 的 SystemPrompt
	}
	for _, o := range opts {
		o(a)
	}
	// 根据是否设置 policy/asker 选择 dispatcher 构造方式
	switch {
	case a.policy != nil && a.asker != nil:
		a.dispatcher = tool.NewDispatcherWithPolicyAndAsker(registry, a.policy, a.asker)
	case a.policy != nil:
		a.dispatcher = tool.NewDispatcherWithPolicy(registry, a.policy)
	default:
		a.dispatcher = tool.NewDispatcher(registry)
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

// SetAsker 运行时注入 asker，同时更新 dispatcher 的 asker。
func (a *Agent) SetAsker(asker permission.Asker) {
	a.asker = asker
	if a.dispatcher != nil {
		a.dispatcher.SetAsker(asker)
	}
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

// Provider 返回 agent 使用的 provider（供 Compactor 复用）。
func (a *Agent) Provider() llm.Provider {
	return a.provider
}

// Model 返回 agent 使用的 model。
func (a *Agent) Model() string {
	return a.model
}

// Run 执行 agent 循环（单轮，无历史保留）。
func (a *Agent) Run(ctx context.Context, userInput string, events chan<- Event) error {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: userInput},
	}
	return a.RunWithHistory(ctx, &messages, events)
}

// RunWithHistory 用已有消息历史执行 agent 循环。
// messages 是指针，agent 会追加 assistant 和 tool 消息。
// 注意：失败后 messages 状态不可信（可能已追加部分 assistant/tool 消息），
// 调用方应丢弃 messages，不要基于失败后的状态继续复用。
// The events channel is closed by Run/RunWithHistory when the loop terminates.
func (a *Agent) RunWithHistory(ctx context.Context, messages *[]llm.Message, events chan<- Event) error {
	defer close(events)

	// usage 提到循环外，让 maxSteps 路径能访问到最后一次的 usage
	var usage llm.Usage

	for step := 0; step < a.maxSteps; step++ {
		req := llm.Request{
			Model:    a.model,
			System:   a.systemPrompt,
			Messages: *messages,
			Tools:    a.tools,
		}

		ch, err := a.provider.Stream(ctx, req)
		if err != nil {
			return fmt.Errorf("step %d stream: %w", step, err)
		}

		var textBuf string
		// 按 ID 累积流式 tool_call delta，避免多 delta 到达时产生重复/截断
		toolAccums := map[string]*llm.ToolCall{}
		var toolOrder []string // 保持首次出现顺序

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
				acc, ok := toolAccums[e.ID]
				if !ok {
					acc = &llm.ToolCall{ID: e.ID, Name: e.Name}
					toolAccums[e.ID] = acc
					toolOrder = append(toolOrder, e.ID)
				}
				if e.Name != "" {
					acc.Name = e.Name
				}
				// ArgsDelta 可能是完整 args（当前 provider 实现）或真增量；
				// 真增量场景下需 append，完整 args 场景下 append 等价于赋值（首次为空）
				acc.Args += e.ArgsDelta
				select {
				case events <- ToolCallEvent{Name: acc.Name, Args: acc.Args}:
				case <-ctx.Done():
					return ctx.Err()
				}
			case llm.Done:
				usage = e.Usage
				if e.StopReason == "max_tokens" || e.StopReason == "length" {
					return fmt.Errorf("step %d truncated (stop_reason=%s), increase max_tokens", step, e.StopReason)
				}
			case llm.Error:
				return fmt.Errorf("step %d stream error: %w", step, e.Err)
			}
		}

		// flatten 累积的 tool calls，保持首次出现顺序
		var toolCalls []llm.ToolCall
		for _, id := range toolOrder {
			toolCalls = append(toolCalls, *toolAccums[id])
		}

		// assistant 消息回灌
		assistantMsg := llm.Message{Role: llm.RoleAssistant, Content: textBuf, ToolCalls: toolCalls}
		*messages = append(*messages, assistantMsg)

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
		if len(results) != len(toolCalls) {
			return fmt.Errorf("step %d: dispatcher returned %d results for %d calls", step, len(results), len(toolCalls))
		}
		for i, r := range results {
			select {
			case events <- ToolResultEvent{Name: r.Name, Result: r.Result}:
			case <-ctx.Done():
				return ctx.Err()
			}
			// 工具结果回灌：用 ToolCallID 关联，provider 层负责序列化为
			// anthropic tool_result block 或 openai role=tool 消息
			*messages = append(*messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    r.Result.Content,
				ToolCallID: toolCalls[i].ID,
			})
		}
	}

	// maxSteps 超出：发送 DoneEvent 让 renderer 能拿到 usage，再返回 error
	select {
	case events <- DoneEvent{Usage: usage}:
	case <-ctx.Done():
	}
	return fmt.Errorf("max steps (%d) exceeded", a.maxSteps)
}
