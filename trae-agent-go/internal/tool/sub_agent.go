// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"fmt"
	"path/filepath"
)

// AgentFactory 定义创建和执行子 Agent 的抽象接口。
// 引入此接口以便在测试中 mock 掉 Agent 的创建，同时避免 tool 包直接依赖 agent/llm 包造成循环导入。
// modelConfig 使用 any 类型，实际使用时由 agent 包侧注入具体的 llm.ModelConfig。
type AgentFactory interface {
	// CreateAndExecute 使用给定的模型配置创建子 Agent 并执行任务，返回执行结果摘要。
	CreateAndExecute(ctx context.Context, modelConfig any, maxSteps int, toolNames []string, workingDir string, task string) (string, error)
}

// SubAgentTool 将子任务委派给新的 Agent 实例，保护主上下文窗口。
type SubAgentTool struct {
	parentModelConfig any
	agentFactory      AgentFactory
}

// NewSubAgentTool 创建 SubAgentTool 实例。
// 注意：默认不设置 AgentFactory 和 ParentModelConfig，需要分别通过 SetAgentFactory 和 SetParentModelConfig 注入。
func NewSubAgentTool() Tool {
	return &SubAgentTool{}
}

// GetName 返回工具名称。
func (s *SubAgentTool) GetName() string {
	return "sub_agent"
}

// GetDescription 返回工具描述。
func (s *SubAgentTool) GetDescription() string {
	return "Delegate a sub-task to a new Agent instance. This protects the main context window by running the sub-task in an isolated Agent with its own context. The sub-agent has access to bash, read_file, grep_search, glob_search, edit, and task_done tools."
}

// GetParameters 返回工具参数定义。
func (s *SubAgentTool) GetParameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "task_description",
			Type:        "string",
			Description: "Complete description of the sub-task to delegate to the sub-agent",
			Required:    true,
		},
		{
			Name:        "working_dir",
			Type:        "string",
			Description: "Absolute path of the working directory for the sub-agent",
			Required:    true,
		},
	}
}

// GetInputSchema 生成 JSON Schema 参数定义。
func (s *SubAgentTool) GetInputSchema() map[string]any {
	return GetInputSchema(s)
}

// Execute 执行子 Agent 委派逻辑。
func (s *SubAgentTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	// 验证 task_description 参数
	taskDescription, ok := args["task_description"].(string)
	if !ok || taskDescription == "" {
		return ToolResult{
			Success: false,
			Error:   "task_description parameter is required",
		}, nil
	}

	// 验证 working_dir 参数
	workingDir, ok := args["working_dir"].(string)
	if !ok || workingDir == "" {
		return ToolResult{
			Success: false,
			Error:   "working_dir parameter is required",
		}, nil
	}

	// 验证 working_dir 是绝对路径
	if !filepath.IsAbs(workingDir) {
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("working_dir must be an absolute path, got: %s", workingDir),
		}, nil
	}

	// 检查 parentModelConfig 是否已设置
	if s.parentModelConfig == nil {
		return ToolResult{
			Success: false,
			Error:   "parent model config is not set, call SetParentModelConfig first",
		}, nil
	}

	// 检查 agentFactory 是否已设置
	if s.agentFactory == nil {
		return ToolResult{
			Success: false,
			Error:   "agent factory is not set, call SetAgentFactory first",
		}, nil
	}

	// 子 Agent 的工具列表（不包含 sub_agent，防止递归）
	subToolNames := []string{"bash", "read_file", "grep_search", "glob_search", "edit", "task_done"}

	// 通过 AgentFactory 创建并执行子 Agent
	result, err := s.agentFactory.CreateAndExecute(ctx, s.parentModelConfig, 30, subToolNames, workingDir, taskDescription)
	if err != nil {
		return ToolResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return ToolResult{
		Success: true,
		Output:  result,
	}, nil
}

// SetParentModelConfig 设置父 Agent 的模型配置。
// config 参数应为 llm.ModelConfig 类型，使用 any 以避免循环导入。
func (s *SubAgentTool) SetParentModelConfig(config any) {
	s.parentModelConfig = config
}

// SetAgentFactory 设置 Agent 工厂实例。
func (s *SubAgentTool) SetAgentFactory(factory AgentFactory) {
	s.agentFactory = factory
}
