// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"

	"github.com/bytedance/trae-agent-go/internal/llm"
)

// DefaultAgentFactory 是 tool.AgentFactory 的默认实现，创建真正的 Agent。
type DefaultAgentFactory struct{}

// CreateAndExecute 创建子 Agent 并执行任务，返回执行结果摘要。
func (f *DefaultAgentFactory) CreateAndExecute(ctx context.Context, modelConfig any, maxSteps int, toolNames []string, workingDir string, task string) (string, error) {
	mc, ok := modelConfig.(llm.ModelConfig)
	if !ok {
		return "", fmt.Errorf("invalid model config type")
	}

	agentConfig := AgentConfig{
		ModelConfig: mc,
		MaxSteps:    maxSteps,
		ToolNames:   toolNames,
		WorkingDir:  workingDir,
	}

	subAgent, err := NewAgent(agentConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create sub agent: %w", err)
	}

	subAgent.NewTask(task, workingDir)

	execution, err := subAgent.ExecuteTask(ctx)
	if err != nil {
		return "", fmt.Errorf("sub agent execution failed: %w", err)
	}

	if execution.Success {
		return fmt.Sprintf("Sub-agent completed successfully. Steps: %d, Tokens: input=%d output=%d, Time: %.1fs",
			len(execution.Steps),
			execution.TotalTokens.InputTokens,
			execution.TotalTokens.OutputTokens,
			execution.ExecutionTime,
		), nil
	}

	return fmt.Sprintf("Sub-agent did not complete successfully. Steps: %d, State: %s",
		len(execution.Steps),
		execution.AgentState,
	), nil
}
