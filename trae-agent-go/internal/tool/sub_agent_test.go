// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"strings"
	"testing"
)

// mockModelConfig 是测试用的模型配置，模拟 llm.ModelConfig 的结构。
type mockModelConfig struct {
	Model    string
	Provider string
	APIKey   string
}

// mockAgentFactory 是测试用的 AgentFactory mock 实现。
type mockAgentFactory struct {
	createAndExecuteCalled bool
	lastModelConfig       any
	lastMaxSteps          int
	lastToolNames         []string
	lastWorkingDir        string
	lastTask              string
	result                string
	err                   error
}

func (m *mockAgentFactory) CreateAndExecute(ctx context.Context, modelConfig any, maxSteps int, toolNames []string, workingDir string, task string) (string, error) {
	m.createAndExecuteCalled = true
	m.lastModelConfig = modelConfig
	m.lastMaxSteps = maxSteps
	m.lastToolNames = toolNames
	m.lastWorkingDir = workingDir
	m.lastTask = task
	if m.err != nil {
		return "", m.err
	}
	return m.result, nil
}

func TestSubAgentToolName(t *testing.T) {
	tool := &SubAgentTool{}
	if tool.GetName() != "sub_agent" {
		t.Errorf("expected name 'sub_agent', got '%s'", tool.GetName())
	}
}

func TestSubAgentToolMissingTaskDescription(t *testing.T) {
	tool := &SubAgentTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"working_dir": "/tmp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing task_description")
	}
	if !strings.Contains(result.Error, "task_description") {
		t.Errorf("expected error about task_description, got: %s", result.Error)
	}
}

func TestSubAgentToolMissingWorkingDir(t *testing.T) {
	tool := &SubAgentTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"task_description": "do something",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing working_dir")
	}
	if !strings.Contains(result.Error, "working_dir") {
		t.Errorf("expected error about working_dir, got: %s", result.Error)
	}
}

func TestSubAgentToolRelativePath(t *testing.T) {
	tool := &SubAgentTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"task_description": "do something",
		"working_dir":      "relative/path",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for relative path")
	}
	if !strings.Contains(result.Error, "absolute path") {
		t.Errorf("expected error about absolute path, got: %s", result.Error)
	}
}

func TestSubAgentToolExecution(t *testing.T) {
	mock := &mockAgentFactory{
		result: "Sub-agent completed successfully. Steps: 3, Tokens: input=100 output=50, Time: 1.5s",
	}

	config := &mockModelConfig{
		Model:    "test-model",
		Provider: "anthropic",
		APIKey:   "test-key",
	}

	tool := &SubAgentTool{
		parentModelConfig: config,
		agentFactory:      mock,
	}

	result, err := tool.Execute(context.Background(), map[string]any{
		"task_description": "fix the bug in main.go",
		"working_dir":      "/home/user/project",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// 验证 mock 被调用
	if !mock.createAndExecuteCalled {
		t.Error("expected CreateAndExecute to be called")
	}

	// 验证传递的参数
	if mock.lastTask != "fix the bug in main.go" {
		t.Errorf("expected task 'fix the bug in main.go', got '%s'", mock.lastTask)
	}
	if mock.lastWorkingDir != "/home/user/project" {
		t.Errorf("expected working_dir '/home/user/project', got '%s'", mock.lastWorkingDir)
	}

	// 验证子 Agent 配置
	if mock.lastMaxSteps != 30 {
		t.Errorf("expected MaxSteps 30, got %d", mock.lastMaxSteps)
	}

	// 验证工具列表不包含 sub_agent
	expectedToolNames := []string{"bash", "read_file", "grep_search", "glob_search", "edit", "task_done"}
	if len(mock.lastToolNames) != len(expectedToolNames) {
		t.Errorf("expected %d tool names, got %d", len(expectedToolNames), len(mock.lastToolNames))
	}
	for i, name := range expectedToolNames {
		if mock.lastToolNames[i] != name {
			t.Errorf("expected tool name[%d] '%s', got '%s'", i, name, mock.lastToolNames[i])
		}
	}
	for _, name := range mock.lastToolNames {
		if name == "sub_agent" {
			t.Error("tool names should not contain 'sub_agent' to prevent recursion")
		}
	}

	// 验证模型配置被正确传递
	cfg, ok := mock.lastModelConfig.(*mockModelConfig)
	if !ok {
		t.Fatalf("expected *mockModelConfig, got %T", mock.lastModelConfig)
	}
	if cfg.Model != "test-model" {
		t.Errorf("expected model 'test-model', got '%s'", cfg.Model)
	}
}
