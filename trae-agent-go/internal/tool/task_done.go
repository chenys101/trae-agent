// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"fmt"
)

// TaskDoneTool 标记当前任务完成。
type TaskDoneTool struct{}

// NewTaskDoneTool 创建 TaskDoneTool 实例。
func NewTaskDoneTool() Tool {
	return &TaskDoneTool{}
}

// GetName 返回工具名称。
func (t *TaskDoneTool) GetName() string {
	return "task_done"
}

// GetDescription 返回工具描述。
func (t *TaskDoneTool) GetDescription() string {
	return "标记当前任务完成"
}

// GetParameters 返回工具参数定义。
func (t *TaskDoneTool) GetParameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "result",
			Type:        "string",
			Description: "任务的最终结果",
			Required:    false,
		},
	}
}

// Execute 执行任务完成操作。
func (t *TaskDoneTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	result, _ := args["result"].(string)
	if result == "" {
		result = "任务已完成"
	}
	return ToolResult{
		Success: true,
		Output:  fmt.Sprintf("任务完成: %s", result),
	}, nil
}
