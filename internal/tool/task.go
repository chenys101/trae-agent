package tool

import (
	"context"
	"encoding/json"
)

// TaskRunner 子 agent 执行接口，由 agent 包实现并注入。
// 解耦 tool → agent 的依赖（避免循环 import）。
type TaskRunner interface {
	RunSubagent(ctx context.Context, subagentType, description, prompt string) (string, error)
}

// Task 工具：派发子 agent 执行子任务。
// 主 agent 通过此工具并行派发多个子 agent，每个子 agent 有独立上下文。
type Task struct {
	runner TaskRunner
}

// NewTask 构造 Task 工具。
func NewTask(runner TaskRunner) *Task {
	return &Task{runner: runner}
}

func (Task) Name() string { return "task" }

func (Task) Description() string {
	return "Launch a new agent to handle complex, multi-step tasks autonomously. " +
		"Use for parallelizing independent queries or cross-layer changes. " +
		"Each sub-agent has its own context window and restricted tool set."
}

// Schema 返回 Task 工具的 JSON Schema。
// 注意：enum 中的子 agent 类型与 agent.subagentTypes 保持同步。
// 此处不能 import agent 包（循环依赖），新增子 agent 类型时需手动同步。
func (Task) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "subagent_type": {
      "type": "string",
      "description": "Type of sub-agent to launch",
      "enum": ["search", "general-purpose"]
    },
    "description": {
      "type": "string",
      "description": "A short (3-5 words) description of the task"
    },
    "prompt": {
      "type": "string",
      "description": "The task for the agent to perform. Provide all necessary context."
    }
  },
  "required": ["subagent_type", "description", "prompt"]
}`)
}

type taskArgs struct {
	SubagentType string `json:"subagent_type"`
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
}

func (t *Task) Run(ctx context.Context, args json.RawMessage) Result {
	var a taskArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.SubagentType == "" {
		return ErrorResult("subagent_type is required")
	}
	if a.Prompt == "" {
		return ErrorResult("prompt is required")
	}

	result, err := t.runner.RunSubagent(ctx, a.SubagentType, a.Description, a.Prompt)
	if err != nil {
		// 子 agent 失败但已产出部分文本时，返回文本 + 警告，而非丢弃
		if result != "" {
			return Result{Content: result + "\n\n[warning: subagent ended with error: " + err.Error() + "]"}
		}
		return ErrorResult("subagent %s failed: %v", a.SubagentType, err)
	}
	return Result{Content: result}
}

// 确保 Task 实现 Tool 接口
var _ Tool = (*Task)(nil)
