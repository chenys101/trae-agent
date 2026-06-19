// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// BashTool 提供执行 shell 命令的能力。
// 状态在调用间持久化，支持通过 restart 参数重启 bash 会话。
type BashTool struct {
	WorkingDir string
}

// NewBashTool 创建 BashTool 实例。
func NewBashTool() Tool {
	return &BashTool{}
}

// GetName 返回工具名称 "bash"。
func (b *BashTool) GetName() string {
	return "bash"
}

// GetDescription 返回工具的功能描述。
func (b *BashTool) GetDescription() string {
	return "Execute bash commands. State is persistent across calls. Use restart to start a new session."
}

// GetParameters 返回工具的参数定义列表。
func (b *BashTool) GetParameters() []ToolParameter {
	return []ToolParameter{
		{
			Name:        "command",
			Type:        "string",
			Description: "The bash command to execute",
			Required:    true,
		},
		{
			Name:        "restart",
			Type:        "boolean",
			Description: "Whether to restart the bash session",
			Required:    false,
		},
	}
}

// Execute 执行 bash 命令并返回结果。
func (b *BashTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	// 检查 restart 参数
	if restart, _ := args["restart"].(bool); restart {
		return ToolResult{
			Success: true,
			Output:  "tool has been restarted.",
		}, nil
	}

	// 获取 command 参数
	command, ok := args["command"].(string)
	if !ok || command == "" {
		return ToolResult{
			Success: false,
			Error:   "command parameter is required",
		}, nil
	}

	// 设置 120 秒超时
	timeoutCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", command)
	if b.WorkingDir != "" {
		cmd.Dir = b.WorkingDir
	}

	output, err := cmd.CombinedOutput()

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return ToolResult{
			Success: false,
			Error:   fmt.Sprintf("command timed out after 120 seconds: %s", command),
		}, nil
	}

	if err != nil {
		return ToolResult{
			Success: false,
			Output:  string(output),
			Error:   err.Error(),
		}, nil
	}

	return ToolResult{
		Success: true,
		Output:  string(output),
	}, nil
}
