// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"testing"
)

// mockTool 是用于测试的模拟工具实现。
type mockTool struct {
	name        string
	description string
	parameters  []ToolParameter
	executeFunc func(ctx context.Context, args map[string]any) (ToolResult, error)
}

func (m *mockTool) GetName() string                          { return m.name }
func (m *mockTool) GetDescription() string                   { return m.description }
func (m *mockTool) GetParameters() []ToolParameter           { return m.parameters }
func (m *mockTool) GetInputSchema() map[string]any           { return GetInputSchema(m) }
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, args)
	}
	return ToolResult{Success: true, Output: "mock result"}, nil
}

func TestToolInterface(t *testing.T) {
	tool := &mockTool{
		name:        "test_tool",
		description: "A test tool",
		parameters: []ToolParameter{
			{Name: "input", Type: "string", Description: "Test input", Required: true},
		},
	}

	if tool.GetName() != "test_tool" {
		t.Errorf("expected name 'test_tool', got '%s'", tool.GetName())
	}
	if tool.GetDescription() != "A test tool" {
		t.Errorf("expected description 'A test tool', got '%s'", tool.GetDescription())
	}
	if len(tool.GetParameters()) != 1 {
		t.Errorf("expected 1 parameter, got %d", len(tool.GetParameters()))
	}
}

func TestToolExecutorExecuteToolCall(t *testing.T) {
	tool := &mockTool{
		name: "echo",
		executeFunc: func(ctx context.Context, args map[string]any) (ToolResult, error) {
			msg, _ := args["message"].(string)
			return ToolResult{Success: true, Output: msg}, nil
		},
	}

	executor := NewToolExecutor([]Tool{tool})

	result := executor.ExecuteToolCall(context.Background(), ToolCall{
		Name:      "echo",
		CallID:    "call-1",
		Arguments: map[string]any{"message": "hello"},
	})

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Output != "hello" {
		t.Errorf("expected output 'hello', got '%s'", result.Output)
	}
}

func TestToolExecutorToolNotFound(t *testing.T) {
	executor := NewToolExecutor([]Tool{})

	result := executor.ExecuteToolCall(context.Background(), ToolCall{
		Name:   "nonexistent",
		CallID: "call-1",
	})

	if result.Success {
		t.Error("expected failure for nonexistent tool")
	}
	if result.Error == "" {
		t.Error("expected error message for nonexistent tool")
	}
}

func TestToolExecutorParallelExecute(t *testing.T) {
	tool := &mockTool{
		name: "echo",
		executeFunc: func(ctx context.Context, args map[string]any) (ToolResult, error) {
			msg, _ := args["message"].(string)
			return ToolResult{Success: true, Output: msg}, nil
		},
	}

	executor := NewToolExecutor([]Tool{tool})

	calls := []ToolCall{
		{Name: "echo", CallID: "call-1", Arguments: map[string]any{"message": "first"}},
		{Name: "echo", CallID: "call-2", Arguments: map[string]any{"message": "second"}},
	}

	results := executor.ParallelExecute(context.Background(), calls)

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	// 结果顺序应与调用顺序一致
	found := map[string]bool{}
	for _, r := range results {
		found[r.Output] = true
		if !r.Success {
			t.Errorf("expected success, got error: %s", r.Error)
		}
	}
	if !found["first"] || !found["second"] {
		t.Errorf("expected outputs 'first' and 'second', got %v", found)
	}
}

func TestToolExecutorSequentialExecute(t *testing.T) {
	tool := &mockTool{
		name: "echo",
		executeFunc: func(ctx context.Context, args map[string]any) (ToolResult, error) {
			msg, _ := args["message"].(string)
			return ToolResult{Success: true, Output: msg}, nil
		},
	}

	executor := NewToolExecutor([]Tool{tool})

	calls := []ToolCall{
		{Name: "echo", CallID: "call-1", Arguments: map[string]any{"message": "first"}},
		{Name: "echo", CallID: "call-2", Arguments: map[string]any{"message": "second"}},
	}

	results := executor.SequentialExecute(context.Background(), calls)

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	if results[0].Output != "first" {
		t.Errorf("expected first result 'first', got '%s'", results[0].Output)
	}
	if results[1].Output != "second" {
		t.Errorf("expected second result 'second', got '%s'", results[1].Output)
	}
}

func TestGetInputSchema(t *testing.T) {
	tool := &mockTool{
		name: "test",
		parameters: []ToolParameter{
			{Name: "path", Type: "string", Description: "File path", Required: true},
			{Name: "verbose", Type: "boolean", Description: "Verbose output", Required: false},
		},
	}

	schema := GetInputSchema(tool)

	if schema["type"] != "object" {
		t.Errorf("expected type 'object', got '%v'", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be map[string]any")
	}
	if len(props) != 2 {
		t.Errorf("expected 2 properties, got %d", len(props))
	}

	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("expected required to be []string")
	}
	if len(required) != 1 || required[0] != "path" {
		t.Errorf("expected required ['path'], got %v", required)
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"bash", "bash"},
		{"str_replace_based_edit_tool", "strreplacebasededittool"},
		{"ReadFile", "readfile"},
		{"JSON_Edit_Tool", "jsonedittool"},
	}

	for _, tt := range tests {
		got := normalizeName(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
