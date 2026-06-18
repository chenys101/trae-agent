// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GlobSearchTool 按 glob 模式查找文件，替代 bash find/ls。
type GlobSearchTool struct{}

// NewGlobSearchTool 创建 GlobSearchTool 实例。
func NewGlobSearchTool() Tool {
	return &GlobSearchTool{}
}

// GetName 返回工具名称。
func (g *GlobSearchTool) GetName() string {
	return "glob_search"
}

// GetDescription 返回工具描述。
func (g *GlobSearchTool) GetDescription() string {
	return "按 glob 模式查找文件，替代 bash find/ls"
}

// GetParameters 返回工具参数定义。
func (g *GlobSearchTool) GetParameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "pattern",
			Type:        "string",
			Description: "glob 模式（如 \"**/*.py\"）",
			Required:    true,
		},
		{
			Name:        "path",
			Type:        "string",
			Description: "搜索的目录绝对路径",
			Required:    true,
		},
	}
}

// GetInputSchema 生成 JSON Schema 参数定义。
func (g *GlobSearchTool) GetInputSchema() map[string]any {
	return GetInputSchema(g)
}

// Execute 执行 glob 搜索。
func (g *GlobSearchTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	pattern, _ := args["pattern"].(string)
	path, _ := args["path"].(string)

	if pattern == "" {
		return ToolResult{Success: false, Error: "pattern 参数必须提供"}, nil
	}
	if path == "" {
		return ToolResult{Success: false, Error: "path 参数必须提供"}, nil
	}

	// 验证路径必须是绝对路径
	if !filepath.IsAbs(path) {
		return ToolResult{Success: false, Error: "path 必须是绝对路径"}, nil
	}

	// 验证路径必须存在且为目录
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ToolResult{Success: false, Error: fmt.Sprintf("路径不存在: %s", path)}, nil
		}
		return ToolResult{Success: false, Error: fmt.Sprintf("无法访问路径: %v", err)}, nil
	}
	if !info.IsDir() {
		return ToolResult{Success: false, Error: fmt.Sprintf("路径不是目录: %s", path)}, nil
	}

	var matchedFiles []string

	err = filepath.WalkDir(path, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		// 获取相对于搜索路径的相对路径用于匹配
		relPath, relErr := filepath.Rel(path, filePath)
		if relErr != nil {
			return nil
		}

		matched, matchErr := filepath.Match(pattern, relPath)
		if matchErr != nil || !matched {
			return nil
		}

		matchedFiles = append(matchedFiles, filePath)
		return nil
	})

	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("遍历目录失败: %v", err)}, nil
	}

	if len(matchedFiles) == 0 {
		return ToolResult{Success: true, Output: "No matches found"}, nil
	}

	output := strings.Join(matchedFiles, "\n")
	output += fmt.Sprintf("\n\n共找到 %d 个匹配文件", len(matchedFiles))

	return ToolResult{Success: true, Output: output}, nil
}
