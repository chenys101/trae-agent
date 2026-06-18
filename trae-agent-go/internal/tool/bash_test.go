// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"strings"
	"testing"
)

func TestBashToolGetName(t *testing.T) {
	tool := &BashTool{}
	if tool.GetName() != "bash" {
		t.Errorf("expected name 'bash', got '%s'", tool.GetName())
	}
}

func TestBashToolEchoHello(t *testing.T) {
	tool := &BashTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if strings.TrimSpace(result.Output) != "hello" {
		t.Errorf("expected output 'hello', got '%s'", strings.TrimSpace(result.Output))
	}
}

func TestBashToolCommandNotFound(t *testing.T) {
	tool := &BashTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"command": "nonexistent_command_xyz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for nonexistent command")
	}
	if result.Error == "" {
		t.Error("expected error message for nonexistent command")
	}
}

func TestBashToolRestart(t *testing.T) {
	tool := &BashTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"restart": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Output != "tool has been restarted." {
		t.Errorf("expected output 'tool has been restarted.', got '%s'", result.Output)
	}
}

func TestBashToolMissingCommand(t *testing.T) {
	tool := &BashTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing command parameter")
	}
	if result.Error != "command parameter is required" {
		t.Errorf("expected error 'command parameter is required', got '%s'", result.Error)
	}
}
