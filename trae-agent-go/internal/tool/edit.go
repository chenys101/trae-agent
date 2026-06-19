// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// EditTool 使用字符串替换编辑文件内容。
type EditTool struct{}

// NewEditTool 创建 EditTool 实例。
func NewEditTool() Tool {
	return &EditTool{}
}

// GetName 返回工具名称。
func (e *EditTool) GetName() string {
	return "edit"
}

// GetDescription 返回工具描述。
func (e *EditTool) GetDescription() string {
	return "使用字符串替换编辑文件内容"
}

// GetParameters 返回工具参数定义。
func (e *EditTool) GetParameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "file_path",
			Type:        "string",
			Description: "要编辑的文件绝对路径",
			Required:    true,
		},
		{
			Name:        "old_string",
			Type:        "string",
			Description: "要替换的原始字符串",
			Required:    true,
		},
		{
			Name:        "new_string",
			Type:        "string",
			Description: "替换后的新字符串",
			Required:    true,
		},
	}
}

// Execute 执行文件编辑操作。
func (e *EditTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	filePath, _ := args["file_path"].(string)
	oldString, _ := args["old_string"].(string)
	newString, _ := args["new_string"].(string)

	if filePath == "" {
		return ToolResult{Success: false, Error: "file_path 参数必须提供"}, nil
	}
	if oldString == "" {
		return ToolResult{Success: false, Error: "old_string 参数必须提供"}, nil
	}
	if !strings.HasPrefix(filePath, "/") {
		return ToolResult{Success: false, Error: "file_path 必须是绝对路径"}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return ToolResult{}, &ToolError{Tool: "edit", Op: "read", Path: filePath, Message: fmt.Sprintf("读取文件失败: %v", err)}
	}

	content := string(data)
	if !strings.Contains(content, oldString) {
		return ToolResult{Success: false, Error: "old_string 未在文件中找到"}, nil
	}

	newContent := strings.Replace(content, oldString, newString, 1)
	if err := os.WriteFile(filePath, []byte(newContent), 0); err != nil {
		return ToolResult{}, &ToolError{Tool: "edit", Op: "write", Path: filePath, Message: fmt.Sprintf("写入文件失败: %v", err)}
	}

	return ToolResult{Success: true, Output: fmt.Sprintf("文件 %s 已更新", filePath)}, nil
}
