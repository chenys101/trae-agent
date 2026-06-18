// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"testing"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := &Registry{
		factories: make(map[string]func() Tool),
	}
	r.Register("test_tool", NewBashTool)

	tool, err := r.Get("test_tool")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tool.GetName() != "bash" {
		t.Errorf("expected tool name 'bash', got '%s'", tool.GetName())
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := &Registry{
		factories: make(map[string]func() Tool),
	}

	_, err := r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent tool, got nil")
	}
}

func TestRegistryList(t *testing.T) {
	r := &Registry{
		factories: make(map[string]func() Tool),
	}
	r.Register("zebra", NewBashTool)
	r.Register("alpha", NewReadFileTool)
	r.Register("middle", NewEditTool)

	names := r.List()
	expected := []string{"alpha", "middle", "zebra"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("expected name[%d] = %q, got %q", i, name, names[i])
		}
	}
}

func TestNewDefaultRegistry(t *testing.T) {
	r := NewDefaultRegistry()

	expectedTools := []string{
		"bash",
		"edit",
		"globsearch",
		"grepsearch",
		"readfile",
		"sequentialthinking",
		"taskdone",
	}

	names := r.List()
	if len(names) != len(expectedTools) {
		t.Fatalf("expected %d tools, got %d: %v", len(expectedTools), len(names), names)
	}

	for _, name := range expectedTools {
		_, err := r.Get(name)
		if err != nil {
			t.Errorf("expected tool %q to be registered, got error: %v", name, err)
		}
	}
}

func TestRegistryNormalizeName(t *testing.T) {
	r := &Registry{
		factories: make(map[string]func() Tool),
	}
	r.Register("My_Tool", NewBashTool)

	// 应该能通过标准化后的名称获取
	tool, err := r.Get("mytool")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tool.GetName() != "bash" {
		t.Errorf("expected tool name 'bash', got '%s'", tool.GetName())
	}
}
