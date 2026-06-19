// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/trae-agent-go/internal/llm"
	"github.com/bytedance/trae-agent-go/internal/prompt"
	"github.com/bytedance/trae-agent-go/internal/tool"
)

const defaultMaxSteps = 30

// AgentConfig 定义 Agent 的配置参数。
type AgentConfig struct {
	ModelConfig llm.ModelConfig
	MaxSteps    int
	ToolNames   []string
	WorkingDir  string
}

// Agent 是核心执行引擎，管理 LLM 交互和工具调用的循环。
type Agent struct {
	client    llm.LLMClient
	executor  *tool.ToolExecutor
	config    AgentConfig
	registry  *tool.Registry
	messages  []llm.LLMMessage
	execution *AgentExecution
}

// NewAgent 根据配置创建一个新的 Agent 实例。
func NewAgent(config AgentConfig) (*Agent, error) {
	if config.MaxSteps <= 0 {
		config.MaxSteps = defaultMaxSteps
	}

	registry := tool.NewDefaultRegistry()

	var tools []tool.Tool
	for _, name := range config.ToolNames {
		t, err := registry.Get(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get tool %q: %w", name, err)
		}
		tools = append(tools, t)
	}

	executor := tool.NewToolExecutor(tools)

	client, err := llm.NewClient(config.ModelConfig.Provider, config.ModelConfig.APIKey, config.ModelConfig.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	return &Agent{
		client:   client,
		executor: executor,
		config:   config,
		registry: registry,
	}, nil
}

// NewTask 重置 Agent 状态并初始化一个新任务。
func (a *Agent) NewTask(task string, projectPath string) {
	a.messages = nil
	a.execution = NewAgentExecution(task)

	a.messages = append(a.messages, llm.LLMMessage{
		Role:    "system",
		Content: prompt.GetSystemPrompt(),
	})

	userContent := task
	if projectPath != "" {
		userContent = InjectProjectContext(task, projectPath)
	}

	a.messages = append(a.messages, llm.LLMMessage{
		Role:    "user",
		Content: userContent,
	})
}

// ExecuteTask 执行 Agent 的核心循环，直到任务完成或达到最大步数。
func (a *Agent) ExecuteTask(ctx context.Context) (*AgentExecution, error) {
	if a.execution == nil {
		return nil, fmt.Errorf("no task initialized, call NewTask first")
	}

	a.execution.AgentState = StateRunning

	maxSteps := a.config.MaxSteps

	for step := 1; step <= maxSteps; step++ {
		err := a.runLLMStep(ctx, step)
		if err != nil {
			a.execution.AgentState = StateError
			return a.execution, fmt.Errorf("step %d failed: %w", step, err)
		}

		// 检查最后一步是否有 task_done
		if len(a.execution.Steps) > 0 {
			lastStep := a.execution.Steps[len(a.execution.Steps)-1]
			if hasTaskDone(lastStep.ToolCalls) {
				a.execution.Success = true
				break
			}
		}

		// 达到最大步数，注入强制总结消息
		if step == maxSteps {
			a.messages = append(a.messages, llm.LLMMessage{
				Role:    "user",
				Content: "You have reached the maximum number of steps. Please use the task_done tool to summarize what you have accomplished so far.",
			})
			err := a.runLLMStep(ctx, maxSteps+1)
			if err != nil {
				a.execution.AgentState = StateError
				return a.execution, fmt.Errorf("final step failed: %w", err)
			}
			// 检查最终步骤是否完成
			if len(a.execution.Steps) > 0 {
				lastStep := a.execution.Steps[len(a.execution.Steps)-1]
				if hasTaskDone(lastStep.ToolCalls) {
					a.execution.Success = true
				}
			}
			break
		}
	}

	a.execution.AgentState = StateCompleted
	a.execution.ExecutionTime = time.Since(a.execution.StartTime).Seconds()

	return a.execution, nil
}

// runLLMStep 执行单步 LLM 交互。
func (a *Agent) runLLMStep(ctx context.Context, stepNumber int) error {
	tools := a.executor.GetTools()

	response, err := a.client.Chat(ctx, llm.ChatRequest{
		Messages: a.messages,
		Config:   a.config.ModelConfig,
		Tools:    tools,
	})
	if err != nil {
		return fmt.Errorf("LLM chat failed: %w", err)
	}

	// 记录 token 使用量
	a.execution.AddTokenUsage(TokenUsage{
		InputTokens:  response.Usage.InputTokens,
		OutputTokens: response.Usage.OutputTokens,
	})

	if len(response.ToolCalls) > 0 {
		// 工具调用步骤
		agentStep := AgentStep{
			StepNumber: stepNumber,
			State:      StepCallingTool,
			Thought:    response.Content,
			ToolCalls:  response.ToolCalls,
		}

		// 执行工具调用
		toolCalls := convertToToolCalls(response.ToolCalls)

		var toolResults []tool.ToolResult
		if a.config.ModelConfig.ParallelToolCalls {
			toolResults = a.executor.ParallelExecute(ctx, toolCalls)
		} else {
			toolResults = a.executor.SequentialExecute(ctx, toolCalls)
		}

		// 反思：检查失败的工具
		agentStep.ToolResults = toolResults
		var reflection string
		for _, tr := range toolResults {
			if !tr.Success {
				reflection += fmt.Sprintf("Tool %s failed: %s\n", tr.Name, tr.Error)
			}
		}
		if reflection != "" {
			agentStep.Reflection = reflection
		}

		a.execution.AddStep(agentStep)

		// 追加 assistant 消息
		a.messages = append(a.messages, llm.LLMMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})

		// 追加 tool result 消息
		for _, tr := range toolResults {
			a.messages = append(a.messages, llm.LLMMessage{
				Role:       "tool",
				ToolCallID: tr.CallID,
				ToolName:   tr.Name,
				ToolResult: tr.Output,
				Content:    tr.Output,
			})
			if !tr.Success {
				a.messages[len(a.messages)-1].Content = fmt.Sprintf("Error: %s", tr.Error)
			}
		}
	} else {
		// 纯思考步骤
		agentStep := AgentStep{
			StepNumber: stepNumber,
			State:      StepThinking,
			Thought:    response.Content,
		}
		a.execution.AddStep(agentStep)

		a.messages = append(a.messages, llm.LLMMessage{
			Role:    "assistant",
			Content: response.Content,
		})
	}

	// 上下文压缩
	if len(a.messages) > 40 {
		a.messages = compressMessages(a.messages)
	}

	return nil
}

// compressMessages 压缩消息列表，保留前 2 条和最后 10 条，中间部分压缩为一条摘要消息。
func compressMessages(messages []llm.LLMMessage) []llm.LLMMessage {
	if len(messages) <= 12 {
		return messages
	}

	head := messages[:2]
	tail := messages[len(messages)-10:]
	compressedCount := len(messages) - 12

	compressed := llm.LLMMessage{
		Role:    "user",
		Content: fmt.Sprintf("[Context compressed: %d earlier messages summarized]", compressedCount),
	}

	result := make([]llm.LLMMessage, 0, len(head)+1+len(tail))
	result = append(result, head...)
	result = append(result, compressed)
	result = append(result, tail...)
	return result
}

// hasTaskDone 检查工具调用列表中是否包含 task_done。
func hasTaskDone(toolCalls []llm.ToolCallInfo) bool {
	for _, tc := range toolCalls {
		if tc.Name == "task_done" {
			return true
		}
	}
	return false
}

// convertToToolCalls 将 llm.ToolCallInfo 转换为 tool.ToolCall。
func convertToToolCalls(calls []llm.ToolCallInfo) []tool.ToolCall {
	result := make([]tool.ToolCall, len(calls))
	for i, c := range calls {
		result[i] = tool.ToolCall{
			Name:      c.Name,
			CallID:    c.CallID,
			Arguments: c.Arguments,
		}
	}
	return result
}
