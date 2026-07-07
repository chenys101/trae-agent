package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bytedance/trae-agent/internal/consts"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

// SubagentType 定义子 agent 的类型，决定其可用工具集和行为约束。
type SubagentType struct {
	Name         string   // 类型名，如 "search"、"general-purpose"
	Description  string   // 给主 agent 看的描述，帮助 LLM 选择合适类型
	AllowedTools []string // 允许使用的工具名；nil 表示不限制（用全部工具）
}

// subagentTypes 内置子 agent 类型。
var subagentTypes = map[string]SubagentType{
	"search": {
		Name:         "search",
		Description:  "Fast agent specialized for exploring codebases. Use for high-level concept searches, finding connections between different parts of the codebase, or searching broad/ambiguous keywords. Only has read-only tools (read/glob/grep), cannot modify files.",
		AllowedTools: []string{"read", "glob", "grep"},
	},
	"general-purpose": {
		Name:         "general-purpose",
		Description:  "Perform a general-purpose coding task (a sub-task of the user's overall task). Use for complex multi-step coding tasks, operations that produce a lot of output not needed after the sub-agent completes, or cross-layer changes that have been planned out and can be implemented independently.",
		AllowedTools: nil, // 不限制，使用全部工具
	},
}

// GetSubagentType 按名称查询子 agent 类型。
func GetSubagentType(name string) (SubagentType, bool) {
	st, ok := subagentTypes[name]
	return st, ok
}

// ListSubagentTypes 返回所有内置子 agent 类型，按 Name 排序保证顺序确定。
func ListSubagentTypes() []SubagentType {
	out := make([]SubagentType, 0, len(subagentTypes))
	for _, st := range subagentTypes {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// RestrictedRegistry 基于全量 registry 构造受限 registry，只保留 AllowedTools 中的工具。
// 若 AllowedTools 为 nil，返回全量 registry 的副本。
func (st SubagentType) RestrictedRegistry(full *tool.Registry) *tool.Registry {
	if st.AllowedTools == nil {
		// 不限制：构造包含全部工具的新 registry
		return tool.NewRegistry(full.List()...)
	}
	allowed := make(map[string]bool, len(st.AllowedTools))
	for _, name := range st.AllowedTools {
		allowed[name] = true
	}
	var kept []tool.Tool
	for _, t := range full.List() {
		if allowed[t.Name()] {
			kept = append(kept, t)
		}
	}
	return tool.NewRegistry(kept...)
}

// SubagentRunner 执行子 agent 任务。
// 子 agent 复用主 agent 的 provider 和工具，但拥有独立的消息历史
// 和受限的工具集（按 SubagentType 过滤）。
type SubagentRunner struct {
	provider llm.Provider
	registry *tool.Registry
	model    string
}

// NewSubagentRunner 构造子 agent 执行器。
func NewSubagentRunner(provider llm.Provider, fullRegistry *tool.Registry, model string) *SubagentRunner {
	return &SubagentRunner{
		provider: provider,
		registry: fullRegistry,
		model:    model,
	}
}

// Run 执行子 agent 任务，返回最终文本结果和可能的 error。
// 子 agent 有独立的消息历史，不污染主 agent。
// 子 agent 的 maxSteps 限制为 10（避免无限循环）。
// 即使出错也可能返回已收集的部分文本，调用方可酌情使用。
func (r *SubagentRunner) Run(ctx context.Context, st SubagentType, prompt string) (string, error) {
	if _, ok := GetSubagentType(st.Name); !ok {
		return "", fmt.Errorf("unknown subagent type: %s", st.Name)
	}

	restrictedReg := st.RestrictedRegistry(r.registry)

	subAgent := New(r.provider, restrictedReg,
		WithMaxSteps(consts.DefaultSubagentMaxSteps),
		WithModel(r.model),
		WithSystemPrompt(subagentSystemPrompt(st, restrictedReg)),
	)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}

	events := make(chan Event, 64)
	var resultText string

	// 用 goroutine 跑 agent，主 goroutine 收集事件。
	// 用 buffered done channel + recover 保证即使 agent panic 也不会死锁。
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("subagent panic: %v", r)
			}
		}()
		done <- subAgent.RunWithHistory(ctx, &messages, events)
	}()

	for ev := range events {
		if te, ok := ev.(TextEvent); ok {
			resultText += te.Content
		}
	}

	// events channel 已关闭（RunWithHistory 的 defer close），等待 done
	err := <-done
	return resultText, err
}

// RunSubagent 实现 tool.TaskRunner 接口，供 Task 工具调用。
// 参数 description 仅用于日志/展示，不传入 LLM。
func (r *SubagentRunner) RunSubagent(ctx context.Context, subagentType, description, prompt string) (string, error) {
	st, ok := GetSubagentType(subagentType)
	if !ok {
		return "", fmt.Errorf("unknown subagent type: %s", subagentType)
	}
	return r.Run(ctx, st, prompt)
}

// subagentSystemPrompt 为子 agent 生成系统提示词，包含其角色描述和可用工具列表。
func subagentSystemPrompt(st SubagentType, reg *tool.Registry) string {
	var toolList []string
	for _, t := range reg.List() {
		toolList = append(toolList, t.Name())
	}
	return fmt.Sprintf(`You are a %s sub-agent. %s

You operate with a restricted tool set and must complete the assigned task autonomously.

Available tools: %s

When you are done, provide a concise summary of your findings or actions as your final text response. This summary will be returned to the parent agent.

Do not ask for user input. Do not reference tools you do not have access to.`, st.Name, st.Description, strings.Join(toolList, ", "))
}
