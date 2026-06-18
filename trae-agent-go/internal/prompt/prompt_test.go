// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package prompt

import (
	"strings"
	"testing"
)

func TestGetSystemPromptNotEmpty(t *testing.T) {
	prompt := GetSystemPrompt()
	if prompt == "" {
		t.Fatal("GetSystemPrompt() returned empty string")
	}
	if len(prompt) < 100 {
		t.Fatalf("GetSystemPrompt() seems too short (%d chars), expected a substantial prompt", len(prompt))
	}
}

func TestGetSystemPromptContainsToolNames(t *testing.T) {
	prompt := GetSystemPrompt()
	tools := []string{
		"read_file",
		"grep_search",
		"glob_search",
		"edit",
		"bash",
		"sequential_thinking",
		"sub_agent",
		"task_done",
	}
	for _, tool := range tools {
		if !strings.Contains(prompt, tool) {
			t.Errorf("GetSystemPrompt() does not contain tool name %q", tool)
		}
	}
}

func TestGetSystemPromptContainsWorkflow(t *testing.T) {
	prompt := GetSystemPrompt()
	steps := []string{
		"Explore",
		"Read",
		"Plan",
		"Edit",
		"Verify",
		"Complete",
	}
	for _, step := range steps {
		if !strings.Contains(prompt, step) {
			t.Errorf("GetSystemPrompt() does not contain workflow step %q", step)
		}
	}
}

func TestBuildUserMessage(t *testing.T) {
	task := "Fix the bug in main.go"
	projectPath := "/home/user/project"
	msg := BuildUserMessage(task, projectPath, "")

	if !strings.Contains(msg, task) {
		t.Errorf("BuildUserMessage() does not contain task description, got: %s", msg)
	}
	if !strings.Contains(msg, projectPath) {
		t.Errorf("BuildUserMessage() does not contain project path, got: %s", msg)
	}
	if strings.Contains(msg, "Project Context") {
		t.Errorf("BuildUserMessage() should not contain project context when empty, got: %s", msg)
	}
}

func TestBuildUserMessageWithProjectContext(t *testing.T) {
	task := "Refactor the auth module"
	projectPath := "/home/user/project"
	projectContext := "Project type: Go\nDirectories: cmd/, internal/\nFiles: go.mod, main.go"
	msg := BuildUserMessage(task, projectPath, projectContext)

	if !strings.Contains(msg, task) {
		t.Errorf("BuildUserMessage() does not contain task description, got: %s", msg)
	}
	if !strings.Contains(msg, projectPath) {
		t.Errorf("BuildUserMessage() does not contain project path, got: %s", msg)
	}
	if !strings.Contains(msg, "Project Context") {
		t.Errorf("BuildUserMessage() does not contain project context section, got: %s", msg)
	}
	if !strings.Contains(msg, projectContext) {
		t.Errorf("BuildUserMessage() does not contain project context content, got: %s", msg)
	}
}
