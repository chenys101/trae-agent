// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/trae-agent-go/internal/llm"
	"github.com/bytedance/trae-agent-go/internal/tool"
)

// mockLLMClient 是 LLMClient 的 mock 实现。
type mockLLMClient struct {
	responses []llm.LLMResponse
	callIndex int
}

func (m *mockLLMClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.LLMResponse, error) {
	if m.callIndex >= len(m.responses) {
		// 默认返回空响应
		return llm.LLMResponse{
			Content:   "no more responses",
			StopReason: "end_turn",
		}, nil
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

// mockTool 是 Tool 接口的 mock 实现。
type mockTool struct {
	name        string
	description string
	executeFunc func(ctx context.Context, args map[string]any) (tool.ToolResult, error)
}

func (m *mockTool) GetName() string                                          { return m.name }
func (m *mockTool) GetDescription() string                                   { return m.description }
func (m *mockTool) GetParameters() []tool.ToolParameter                      { return nil }
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (tool.ToolResult, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, args)
	}
	return tool.ToolResult{Success: true, Output: "mock result"}, nil
}

func TestAgentNewTask(t *testing.T) {
	agent := &Agent{
		client:   &mockLLMClient{},
		executor: tool.NewToolExecutor(nil),
		config:   AgentConfig{MaxSteps: 10},
		registry: tool.NewDefaultRegistry(),
	}

	agent.NewTask("fix the bug", "/tmp/project")

	if len(agent.messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(agent.messages))
	}
	if agent.messages[0].Role != "system" {
		t.Errorf("expected first message role 'system', got '%s'", agent.messages[0].Role)
	}
	if agent.messages[1].Role != "user" {
		t.Errorf("expected second message role 'user', got '%s'", agent.messages[1].Role)
	}
	if agent.execution == nil {
		t.Fatal("expected execution to be initialized")
	}
	if agent.execution.Task != "fix the bug" {
		t.Errorf("expected task 'fix the bug', got '%s'", agent.execution.Task)
	}
	if agent.execution.AgentState != StateIdle {
		t.Errorf("expected state idle, got '%s'", agent.execution.AgentState)
	}
}

func TestAgentExecuteTask_CompletesWithTaskDone(t *testing.T) {
	mockClient := &mockLLMClient{
		responses: []llm.LLMResponse{
			{
				Content: "I will complete the task",
				ToolCalls: []llm.ToolCallInfo{
					{
						Name:   "task_done",
						CallID: "call_1",
						Arguments: map[string]any{
							"result": "task completed successfully",
						},
					},
				},
				Usage:      llm.LLMUsage{InputTokens: 100, OutputTokens: 50},
				StopReason: "tool_use",
			},
		},
	}

	agent := &Agent{
		client:   mockClient,
		executor: tool.NewToolExecutor([]tool.Tool{&mockTool{name: "task_done"}}),
		config:   AgentConfig{MaxSteps: 10, ModelConfig: llm.ModelConfig{Provider: "mock"}},
		registry: tool.NewDefaultRegistry(),
	}

	agent.NewTask("do something", "")

	execution, err := agent.ExecuteTask(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !execution.Success {
		t.Error("expected execution to succeed")
	}
	if execution.AgentState != StateCompleted {
		t.Errorf("expected state completed, got '%s'", execution.AgentState)
	}
	if len(execution.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(execution.Steps))
	}
	if execution.Steps[0].State != StepCallingTool {
		t.Errorf("expected step state calling_tool, got '%s'", execution.Steps[0].State)
	}
	if execution.TotalTokens == nil {
		t.Fatal("expected total tokens to be set")
	}
	if execution.TotalTokens.InputTokens != 100 {
		t.Errorf("expected input tokens 100, got %d", execution.TotalTokens.InputTokens)
	}
}

func TestAgentExecuteTask_ReachesMaxSteps(t *testing.T) {
	// LLM 始终返回纯文本响应，不调用 task_done
	mockClient := &mockLLMClient{
		responses: []llm.LLMResponse{
			{Content: "thinking...", Usage: llm.LLMUsage{InputTokens: 10, OutputTokens: 5}, StopReason: "end_turn"},
			{Content: "still thinking...", Usage: llm.LLMUsage{InputTokens: 10, OutputTokens: 5}, StopReason: "end_turn"},
			{Content: "more thinking...", Usage: llm.LLMUsage{InputTokens: 10, OutputTokens: 5}, StopReason: "end_turn"},
			// 最后一步强制总结后 LLM 仍然不返回 task_done
			{Content: "final answer", Usage: llm.LLMUsage{InputTokens: 10, OutputTokens: 5}, StopReason: "end_turn"},
		},
	}

	agent := &Agent{
		client:   mockClient,
		executor: tool.NewToolExecutor(nil),
		config:   AgentConfig{MaxSteps: 3, ModelConfig: llm.ModelConfig{Provider: "mock"}},
		registry: tool.NewDefaultRegistry(),
	}

	agent.NewTask("do something", "")

	execution, err := agent.ExecuteTask(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if execution.Success {
		t.Error("expected execution to not succeed (no task_done)")
	}
	if execution.AgentState != StateCompleted {
		t.Errorf("expected state completed, got '%s'", execution.AgentState)
	}
	// 3 个正常步骤 + 1 个强制总结步骤 = 4
	if len(execution.Steps) != 4 {
		t.Errorf("expected 4 steps (3 normal + 1 forced), got %d", len(execution.Steps))
	}
}

func TestAgentCompressMessages(t *testing.T) {
	// 创建 50 条消息
	messages := make([]llm.LLMMessage, 50)
	for i := range messages {
		messages[i] = llm.LLMMessage{
			Role:    "user",
			Content: "message " + string(rune('A'+i)),
		}
	}

	compressed := compressMessages(messages)

	// 2 (head) + 1 (compressed) + 10 (tail) = 13
	if len(compressed) != 13 {
		t.Fatalf("expected 13 messages after compression, got %d", len(compressed))
	}

	// 前 2 条保持不变
	if compressed[0].Content != messages[0].Content {
		t.Errorf("expected first message preserved, got '%s'", compressed[0].Content)
	}
	if compressed[1].Content != messages[1].Content {
		t.Errorf("expected second message preserved, got '%s'", compressed[1].Content)
	}

	// 中间压缩消息
	if compressed[2].Role != "user" {
		t.Errorf("expected compressed message role 'user', got '%s'", compressed[2].Role)
	}
	expectedCompressed := "[Context compressed: 38 earlier messages summarized]"
	if compressed[2].Content != expectedCompressed {
		t.Errorf("expected compressed message '%s', got '%s'", expectedCompressed, compressed[2].Content)
	}

	// 最后 10 条保持不变
	for i := 0; i < 10; i++ {
		originalIdx := 40 + i
		if compressed[3+i].Content != messages[originalIdx].Content {
			t.Errorf("expected tail message %d preserved, got '%s'", i, compressed[3+i].Content)
		}
	}
}

func TestAgentCompressMessages_ShortList(t *testing.T) {
	// 少于 12 条消息不应压缩
	messages := make([]llm.LLMMessage, 10)
	for i := range messages {
		messages[i] = llm.LLMMessage{Role: "user", Content: "msg"}
	}

	compressed := compressMessages(messages)
	if len(compressed) != 10 {
		t.Errorf("expected 10 messages (no compression), got %d", len(compressed))
	}
}

func TestGetProjectContext(t *testing.T) {
	// 创建临时项目目录
	tmpDir := t.TempDir()

	// 创建一些文件和目录
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755) // 应该被忽略
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte("hidden"), 0644) // 应该被忽略

	ctx := GetProjectContext(tmpDir)

	if ctx == "" {
		t.Fatal("expected non-empty project context")
	}

	// 应该检测到 Go 项目
	if !contains(ctx, "Go") {
		t.Error("expected project type 'Go' to be detected")
	}

	// 应该包含 src 目录
	if !contains(ctx, "src/") {
		t.Error("expected 'src/' directory in context")
	}

	// 应该包含 go.mod 和 main.go
	if !contains(ctx, "go.mod") {
		t.Error("expected 'go.mod' file in context")
	}
	if !contains(ctx, "main.go") {
		t.Error("expected 'main.go' file in context")
	}

	// 不应该包含隐藏文件/目录
	if contains(ctx, ".git") {
		t.Error("expected '.git' to be excluded from context")
	}
	if contains(ctx, ".hidden") {
		t.Error("expected '.hidden' to be excluded from context")
	}
}

func TestGetProjectContext_NodeJS(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte("{}"), 0644)

	ctx := GetProjectContext(tmpDir)
	if !contains(ctx, "Node.js") {
		t.Error("expected project type 'Node.js' to be detected")
	}
}

func TestGetProjectContext_Python(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[project]"), 0644)

	ctx := GetProjectContext(tmpDir)
	if !contains(ctx, "Python") {
		t.Error("expected project type 'Python' to be detected")
	}
}

func TestGetProjectContext_Rust(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]"), 0644)

	ctx := GetProjectContext(tmpDir)
	if !contains(ctx, "Rust") {
		t.Error("expected project type 'Rust' to be detected")
	}
}

func TestInjectProjectContext(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	result := InjectProjectContext("fix the bug", tmpDir)

	if !contains(result, "fix the bug") {
		t.Error("expected original user message to be preserved")
	}
	if !contains(result, "--- Project Context ---") {
		t.Error("expected project context separator")
	}
	if !contains(result, "Go") {
		t.Error("expected project type in injected context")
	}
}

// contains 检查字符串是否包含子串。
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
