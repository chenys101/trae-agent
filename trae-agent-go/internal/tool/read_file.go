// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ReadFileTool 读取文件内容的工具，支持行号和行范围，替代 bash cat/head。
type ReadFileTool struct{}

// NewReadFileTool 创建 ReadFileTool 实例。
func NewReadFileTool() Tool {
	return &ReadFileTool{}
}

// GetName 返回工具名称。
func (r *ReadFileTool) GetName() string {
	return "read_file"
}

// GetDescription 返回工具描述。
func (r *ReadFileTool) GetDescription() string {
	return "读取文件内容，支持行号和行范围，替代 bash cat/head"
}

// GetParameters 返回工具参数定义。
func (r *ReadFileTool) GetParameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "file_path",
			Type:        "string",
			Description: "文件的绝对路径",
			Required:    true,
		},
		{
			Name:        "offset",
			Type:        "integer",
			Description: "起始行号（1-based），默认 1",
			Required:    false,
		},
		{
			Name:        "limit",
			Type:        "integer",
			Description: "最大读取行数，默认读取整个文件",
			Required:    false,
		},
	}
}

// Execute 执行文件读取操作。
func (r *ReadFileTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	// 验证 file_path 必须提供且为字符串
	filePathVal, ok := args["file_path"]
	if !ok || filePathVal == nil {
		return ToolResult{Success: false, Error: "file_path 参数必须提供"}, nil
	}
	filePath, ok := filePathVal.(string)
	if !ok {
		return ToolResult{Success: false, Error: "file_path 必须是字符串类型"}, nil
	}

	// 验证路径必须是绝对路径
	if !strings.HasPrefix(filePath, "/") {
		return ToolResult{Success: false, Error: "file_path 必须是绝对路径（以 / 开头）"}, nil
	}

	// 验证文件必须存在
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolResult{}, &ToolError{Tool: "read_file", Op: "read", Path: filePath, Message: "文件不存在"}
		}
		return ToolResult{}, &ToolError{Tool: "read_file", Op: "read", Path: filePath, Message: fmt.Sprintf("无法访问文件: %v", err)}
	}

	// 验证路径不能是目录
	if info.IsDir() {
		return ToolResult{}, &ToolError{Tool: "read_file", Op: "read", Path: filePath, Message: "路径是目录，不是文件"}
	}

	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ToolResult{}, &ToolError{Tool: "read_file", Op: "read", Path: filePath, Message: fmt.Sprintf("读取文件失败: %v", err)}
	}

	// 按行分割
	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	// 处理文件末尾空行的情况
	if totalLines > 0 && lines[totalLines-1] == "" {
		totalLines--
		lines = lines[:totalLines]
	}

	// 解析 offset 参数，默认 1
	offset := 1
	if offsetVal, ok := args["offset"]; ok && offsetVal != nil {
		switch v := offsetVal.(type) {
		case float64:
			offset = int(v)
		case int:
			offset = v
		case json.Number:
			n, err := v.Int64()
			if err == nil {
				offset = int(n)
			}
		}
	}

	// 解析 limit 参数，默认读取整个文件
	limit := totalLines
	if limitVal, ok := args["limit"]; ok && limitVal != nil {
		switch v := limitVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case json.Number:
			n, err := v.Int64()
			if err == nil {
				limit = int(n)
			}
		}
	}

	// 应用 offset（1-based 转换为 0-based）
	startIdx := offset - 1
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= totalLines {
		startIdx = totalLines
	}

	endIdx := startIdx + limit
	if endIdx > totalLines {
		endIdx = totalLines
	}

	// 构建输出
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s (%d行)\n", filePath, totalLines))

	for i := startIdx; i < endIdx; i++ {
		sb.WriteString(fmt.Sprintf("%d\t%s\n", i+1, lines[i]))
	}

	output := sb.String()

	// 超长输出截断
	const maxOutputLen = 16000
	if len(output) > maxOutputLen {
		output = output[:maxOutputLen] + "\n...输出已截断，文件内容过长"
	}

	return ToolResult{Success: true, Output: output}, nil
}
