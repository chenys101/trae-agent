// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
)

// baseTool 提供工具的通用基础实现。
type baseTool struct {
	name        string
	description string
	parameters  []ToolParameter
}

func (b *baseTool) GetName() string                { return b.name }
func (b *baseTool) GetDescription() string          { return b.description }
func (b *baseTool) GetParameters() []ToolParameter  { return b.parameters }
func (b *baseTool) GetInputSchema() map[string]any  { return GetInputSchema(b) }
func (b *baseTool) Execute(_ context.Context, _ map[string]any) (ToolResult, error) {
	return ToolResult{Success: true, Output: "not implemented"}, nil
}
