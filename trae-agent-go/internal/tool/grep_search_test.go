// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepSearchToolName(t *testing.T) {
	tool := &GrepSearchTool{}
	if tool.GetName() != "grep_search" {
		t.Errorf("expected name 'grep_search', got '%s'", tool.GetName())
	}
}

func TestGrepSearchBasicMatch(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "hello world\nfoo bar\nhello golang\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &GrepSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Errorf("expected output to contain 'hello world', got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "hello golang") {
		t.Errorf("expected output to contain 'hello golang', got: %s", result.Output)
	}
	if strings.Contains(result.Output, "foo bar") {
		t.Errorf("expected output not to contain 'foo bar', got: %s", result.Output)
	}
}

func TestGrepSearchNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &GrepSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "nonexistent_pattern",
		"path":    tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.Output != "No matches found" {
		t.Errorf("expected 'No matches found', got: %s", result.Output)
	}
}

func TestGrepSearchMissingPattern(t *testing.T) {
	tool := &GrepSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"path": "/tmp",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing pattern")
	}
	if !strings.Contains(result.Error, "pattern") {
		t.Errorf("expected error about pattern, got: %s", result.Error)
	}
}

func TestGrepSearchMissingPath(t *testing.T) {
	tool := &GrepSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing path")
	}
	if !strings.Contains(result.Error, "path") {
		t.Errorf("expected error about path, got: %s", result.Error)
	}
}

func TestGrepSearchNonexistentPath(t *testing.T) {
	tool := &GrepSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "test",
		"path":    "/nonexistent/path/that/does/not/exist",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for nonexistent path")
	}
}

func TestGrepSearchWithInclude(t *testing.T) {
	tmpDir := t.TempDir()
	// 创建 .py 和 .txt 文件
	pyFile := filepath.Join(tmpDir, "test.py")
	txtFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(pyFile, []byte("import os\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := os.WriteFile(txtFile, []byte("import sys\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &GrepSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "import",
		"path":    tmpDir,
		"include": "*.py",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "test.py") {
		t.Errorf("expected output to contain 'test.py', got: %s", result.Output)
	}
	if strings.Contains(result.Output, "test.txt") {
		t.Errorf("expected output not to contain 'test.txt', got: %s", result.Output)
	}
}

func TestGrepSearchCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("Hello World\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &GrepSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern":           "hello",
		"path":              tmpDir,
		"case_insensitive":  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Hello World") {
		t.Errorf("expected output to contain 'Hello World' (case insensitive), got: %s", result.Output)
	}
}
