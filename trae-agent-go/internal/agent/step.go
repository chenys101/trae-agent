// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

// Package agent 定义了 Agent 执行循环的核心数据结构。
package agent

import (
	"time"

	"github.com/bytedance/trae-agent-go/internal/llm"
	"github.com/bytedance/trae-agent-go/internal/tool"
)

// AgentStepState 表示 Agent 单步执行的状态。
type AgentStepState string

const (
	StepThinking    AgentStepState = "thinking"
	StepCallingTool AgentStepState = "calling_tool"
	StepReflecting  AgentStepState = "reflecting"
	StepCompleted   AgentStepState = "completed"
	StepError       AgentStepState = "error"
)

// AgentState 表示 Agent 整体的执行状态。
type AgentState string

const (
	StateIdle      AgentState = "idle"
	StateRunning   AgentState = "running"
	StateCompleted AgentState = "completed"
	StateError     AgentState = "error"
)

// LLMResponseInfo 记录 LLM 的响应信息。
type LLMResponseInfo struct {
	Content    string           `json:"content"`
	ToolCalls  []llm.ToolCallInfo `json:"tool_calls,omitempty"`
	StopReason string           `json:"stop_reason"`
}

// AgentStep 表示 Agent 执行过程中的单个步骤。
type AgentStep struct {
	StepNumber  int              `json:"step_number"`
	State       AgentStepState   `json:"state"`
	Thought     string           `json:"thought,omitempty"`
	ToolCalls   []llm.ToolCallInfo `json:"tool_calls,omitempty"`
	ToolResults []tool.ToolResult  `json:"tool_results,omitempty"`
	LLMResponse *LLMResponseInfo `json:"llm_response,omitempty"`
	Reflection  string           `json:"reflection,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// TokenUsage 记录 LLM 的 token 使用量。
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AgentExecution 封装了 Agent 任务的完整执行结果。
type AgentExecution struct {
	Task          string        `json:"task"`
	Steps         []AgentStep   `json:"steps"`
	FinalResult   string        `json:"final_result,omitempty"`
	Success       bool          `json:"success"`
	TotalTokens   *TokenUsage   `json:"total_tokens,omitempty"`
	ExecutionTime float64       `json:"execution_time"`
	AgentState    AgentState    `json:"agent_state"`
	StartTime     time.Time     `json:"start_time"`
}

// NewAgentExecution 创建一个新的 AgentExecution 实例。
func NewAgentExecution(task string) *AgentExecution {
	return &AgentExecution{
		Task:       task,
		Steps:      []AgentStep{},
		AgentState: StateIdle,
		StartTime:  time.Now(),
	}
}

// AddStep 向执行结果中添加一个步骤。
func (e *AgentExecution) AddStep(step AgentStep) {
	e.Steps = append(e.Steps, step)
}

// AddTokenUsage 累加 token 使用量。
func (e *AgentExecution) AddTokenUsage(usage TokenUsage) {
	if e.TotalTokens == nil {
		e.TotalTokens = &TokenUsage{}
	}
	e.TotalTokens.InputTokens += usage.InputTokens
	e.TotalTokens.OutputTokens += usage.OutputTokens
}
