// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"fmt"
)

// SequentialThinkingTool 进行顺序思考推理。
type SequentialThinkingTool struct{}

// NewSequentialThinkingTool 创建 SequentialThinkingTool 实例。
func NewSequentialThinkingTool() Tool {
	return &SequentialThinkingTool{}
}

// GetName 返回工具名称。
func (s *SequentialThinkingTool) GetName() string {
	return "sequential_thinking"
}

// GetDescription 返回工具描述。
func (s *SequentialThinkingTool) GetDescription() string {
	return "进行顺序思考推理步骤"
}

// GetParameters 返回工具参数定义。
func (s *SequentialThinkingTool) GetParameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "thought",
			Type:        "string",
			Description: "当前思考内容",
			Required:    true,
		},
		{
			Name:        "next_thought_needed",
			Type:        "boolean",
			Description: "是否还需要继续思考",
			Required:    true,
		},
	}
}

// GetInputSchema 生成 JSON Schema 参数定义。
func (s *SequentialThinkingTool) GetInputSchema() map[string]any {
	return GetInputSchema(s)
}

// Execute 执行思考步骤。
func (s *SequentialThinkingTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	thought, _ := args["thought"].(string)
	if thought == "" {
		return ToolResult{Success: false, Error: "thought 参数必须提供"}, nil
	}

	nextThoughtNeeded, _ := args["next_thought_needed"].(bool)
	status := "思考完成"
	if nextThoughtNeeded {
		status = "继续思考"
	}

	return ToolResult{
		Success: true,
		Output:  fmt.Sprintf("%s: %s", status, thought),
	}, nil
}
