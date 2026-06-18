// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// GrepSearchTool 使用正则表达式搜索文件内容，优先使用 ripgrep，替代 bash grep。
type GrepSearchTool struct{}

// NewGrepSearchTool 创建 GrepSearchTool 实例。
func NewGrepSearchTool() Tool {
	return &GrepSearchTool{}
}

// GetName 返回工具名称。
func (g *GrepSearchTool) GetName() string {
	return "grep_search"
}

// GetDescription 返回工具描述。
func (g *GrepSearchTool) GetDescription() string {
	return "使用正则表达式搜索文件内容，优先使用 ripgrep，替代 bash grep"
}

// GetParameters 返回工具参数定义。
func (g *GrepSearchTool) GetParameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "pattern",
			Type:        "string",
			Description: "正则表达式模式",
			Required:    true,
		},
		{
			Name:        "path",
			Type:        "string",
			Description: "搜索的目录或文件绝对路径",
			Required:    true,
		},
		{
			Name:        "include",
			Type:        "string",
			Description: "文件 glob 过滤模式（如 \"*.py\"）",
			Required:    false,
		},
		{
			Name:        "case_insensitive",
			Type:        "boolean",
			Description: "是否大小写不敏感搜索",
			Required:    false,
		},
	}
}

// GetInputSchema 生成 JSON Schema 参数定义。
func (g *GrepSearchTool) GetInputSchema() map[string]any {
	return GetInputSchema(g)
}

// Execute 执行 grep 搜索。
func (g *GrepSearchTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	pattern, _ := args["pattern"].(string)
	path, _ := args["path"].(string)
	include, _ := args["include"].(string)
	caseInsensitive, _ := args["case_insensitive"].(bool)

	if pattern == "" {
		return ToolResult{Success: false, Error: "pattern 参数必须提供"}, nil
	}
	if path == "" {
		return ToolResult{Success: false, Error: "path 参数必须提供"}, nil
	}

	// 优先使用 ripgrep
	if rgPath, err := exec.LookPath("rg"); err == nil {
		return g.executeWithRg(ctx, rgPath, pattern, path, include, caseInsensitive)
	}

	// 回退到 Go 内置实现
	return g.executeWithGo(pattern, path, include, caseInsensitive)
}

// executeWithRg 使用 ripgrep 执行搜索。
func (g *GrepSearchTool) executeWithRg(ctx context.Context, rgPath, pattern, path, include string, caseInsensitive bool) (ToolResult, error) {
	cmdArgs := []string{"--line-number", "--no-heading"}

	if caseInsensitive {
		cmdArgs = append(cmdArgs, "-i")
	}
	if include != "" {
		cmdArgs = append(cmdArgs, "--glob", include)
	}

	cmdArgs = append(cmdArgs, pattern, path)

	cmd := exec.CommandContext(ctx, rgPath, cmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		// rg 在无匹配时返回退出码 1，不是真正的错误
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return ToolResult{Success: true, Output: "No matches found"}, nil
		}
		return ToolResult{Success: false, Error: fmt.Sprintf("rg 执行失败: %v", err)}, nil
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return ToolResult{Success: true, Output: "No matches found"}, nil
	}

	return ToolResult{Success: true, Output: result}, nil
}

// executeWithGo 使用 Go 内置实现执行搜索。
func (g *GrepSearchTool) executeWithGo(pattern, searchPath, include string, caseInsensitive bool) (ToolResult, error) {
	patternStr := pattern
	if caseInsensitive {
		patternStr = "(?i)" + pattern
	}
	re, err := regexp.Compile(patternStr)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("正则表达式编译失败: %v", err)}, nil
	}

	var matches []string

	err = filepath.WalkDir(searchPath, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		// 应用 include 过滤
		if include != "" {
			matched, matchErr := filepath.Match(include, d.Name())
			if matchErr != nil || !matched {
				return nil
			}
		}

		file, err := os.Open(filePath)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", filePath, lineNum, line))
			}
		}

		return nil
	})

	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("遍历目录失败: %v", err)}, nil
	}

	if len(matches) == 0 {
		return ToolResult{Success: true, Output: "No matches found"}, nil
	}

	return ToolResult{Success: true, Output: strings.Join(matches, "\n")}, nil
}
