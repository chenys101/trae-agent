// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

// Package tool 定义了 Agent 工具系统的核心接口和数据结构。
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// Tool 是所有 Agent 工具的核心接口。
// 每个工具必须实现此接口才能被 Agent 调用。
type Tool interface {
	// GetName 返回工具的唯一名称标识。
	GetName() string

	// GetDescription 返回工具的功能描述，供 LLM 理解工具用途。
	GetDescription() string

	// GetParameters 返回工具的参数定义列表。
	GetParameters() []ToolParameter

	// Execute 执行工具逻辑，返回执行结果。
	Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}

// ToolParameter 定义工具的单个参数。
type ToolParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum,omitempty"`
	Items       *ItemsDef `json:"items,omitempty"`
}

// ItemsDef 定义数组类型参数的元素结构。
type ItemsDef struct {
	Type string `json:"type"`
}

// ToolCall 表示 LLM 发起的一次工具调用请求。
type ToolCall struct {
	Name      string         `json:"name"`
	CallID    string         `json:"call_id"`
	Arguments map[string]any `json:"arguments"`
	ID        string         `json:"id,omitempty"` // OpenAI 特有字段
}

// ToolResult 表示工具执行的返回结果。
type ToolResult struct {
	CallID  string `json:"call_id"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	ID      string `json:"id,omitempty"` // OpenAI 特有字段
}

// ToolError represents an error from a tool execution.
type ToolError struct {
	Tool    string // tool name
	Op      string // operation that failed (e.g. "read", "write", "search")
	Path    string // file path involved, if any
	Message string // human-readable error description
}

func (e *ToolError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s %s: %s", e.Tool, e.Op, e.Path, e.Message)
	}
	return fmt.Sprintf("%s: %s: %s", e.Tool, e.Op, e.Message)
}

// ToolExecutor manages tool registration and execution.
// ToolExecutor is safe for concurrent use: ExecuteToolCall can be called
// from multiple goroutines simultaneously. ParallelExecute uses goroutines
// internally and is also safe.
type ToolExecutor struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewToolExecutor 创建一个新的工具执行器。
func NewToolExecutor(tools []Tool) *ToolExecutor {
	te := &ToolExecutor{
		tools: make(map[string]Tool),
	}
	for _, t := range tools {
		te.tools[normalizeName(t.GetName())] = t
	}
	return te
}

// ExecuteToolCall 执行单个工具调用。
func (te *ToolExecutor) ExecuteToolCall(ctx context.Context, call ToolCall) ToolResult {
	te.mu.RLock()
	t, ok := te.tools[normalizeName(call.Name)]
	te.mu.RUnlock()

	if !ok {
		available := make([]string, 0, len(te.tools))
		te.mu.RLock()
		for _, tool := range te.tools {
			available = append(available, tool.GetName())
		}
		te.mu.RUnlock()
		return ToolResult{
			Name:    call.Name,
			CallID:  call.CallID,
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("tool '%s' not found. available tools: %v", call.Name, available),
		}
	}

	result, err := t.Execute(ctx, call.Arguments)
	if err != nil {
		return ToolResult{
			Name:    call.Name,
			CallID:  call.CallID,
			ID:      call.ID,
			Success: false,
			Error:   fmt.Sprintf("error executing tool '%s': %v", call.Name, err),
		}
	}
	result.CallID = call.CallID
	result.Name = call.Name
	result.ID = call.ID
	return result
}

// ParallelExecute 并行执行多个工具调用。
func (te *ToolExecutor) ParallelExecute(ctx context.Context, calls []ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c ToolCall) {
			defer wg.Done()
			results[idx] = te.ExecuteToolCall(ctx, c)
		}(i, call)
	}
	wg.Wait()
	return results
}

// SequentialExecute 顺序执行多个工具调用。
func (te *ToolExecutor) SequentialExecute(ctx context.Context, calls []ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	for i, call := range calls {
		results[i] = te.ExecuteToolCall(ctx, call)
	}
	return results
}

// GetTools 返回所有已注册的工具列表。
func (te *ToolExecutor) GetTools() []Tool {
	te.mu.RLock()
	defer te.mu.RUnlock()
	tools := make([]Tool, 0, len(te.tools))
	for _, t := range te.tools {
		tools = append(tools, t)
	}
	return tools
}

// GetInputSchema 生成工具的 JSON Schema 参数定义。
func GetInputSchema(t Tool) map[string]any {
	schema := map[string]any{
		"type": "object",
	}

	properties := make(map[string]any)
	var required []string

	for _, param := range t.GetParameters() {
		paramSchema := map[string]any{
			"type":        param.Type,
			"description": param.Description,
		}
		if len(param.Enum) > 0 {
			paramSchema["enum"] = param.Enum
		}
		if param.Items != nil {
			paramSchema["items"] = param.Items
		}
		if param.Required {
			required = append(required, param.Name)
		}
		properties[param.Name] = paramSchema
	}

	schema["properties"] = properties
	if len(required) > 0 {
		schema["required"] = required
	}

	return schema
}

// ToJSONDefinition 生成工具的完整 JSON 定义，用于 LLM function calling。
func ToJSONDefinition(t Tool) map[string]any {
	return map[string]any{
		"name":        t.GetName(),
		"description": t.GetDescription(),
		"parameters":  GetInputSchema(t),
	}
}

// ToJSONDefinitions 批量生成工具的 JSON 定义。
func ToJSONDefinitions(tools []Tool) []map[string]any {
	defs := make([]map[string]any, len(tools))
	for i, t := range tools {
		defs[i] = ToJSONDefinition(t)
	}
	return defs
}

// MarshalArguments 将工具调用参数序列化为 JSON 字符串。
func MarshalArguments(args map[string]any) string {
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// normalizeName 将工具名称标准化为小写并移除下划线。
func normalizeName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		result = append(result, c)
	}
	return string(result)
}
