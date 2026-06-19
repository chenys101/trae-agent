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

func TestGlobSearchToolName(t *testing.T) {
	tool := &GlobSearchTool{}
	if tool.GetName() != "glob_search" {
		t.Errorf("expected name 'glob_search', got '%s'", tool.GetName())
	}
}

func TestGlobSearchBasicMatch(t *testing.T) {
	tmpDir := t.TempDir()
	// 创建一些文件
	if err := os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "b.py"), []byte("b"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "c.txt"), []byte("c"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &GlobSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "a.txt") {
		t.Errorf("expected output to contain 'a.txt', got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "c.txt") {
		t.Errorf("expected output to contain 'c.txt', got: %s", result.Output)
	}
	if strings.Contains(result.Output, "b.py") {
		t.Errorf("expected output not to contain 'b.py', got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "共找到 2 个匹配文件") {
		t.Errorf("expected output to contain match count, got: %s", result.Output)
	}
}

func TestGlobSearchNoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &GlobSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
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

func TestGlobSearchMissingPattern(t *testing.T) {
	tool := &GlobSearchTool{}
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

func TestGlobSearchMissingPath(t *testing.T) {
	tool := &GlobSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
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

func TestGlobSearchNonexistentPath(t *testing.T) {
	tool := &GlobSearchTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    "/nonexistent/path/that/does/not/exist",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	toolErr, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected *ToolError, got %T: %v", err, err)
	}
	if !strings.Contains(toolErr.Message, "不存在") {
		t.Errorf("error message should mention nonexistent path, got: %s", toolErr.Message)
	}
}

func TestGlobSearchRelativePath(t *testing.T) {
	tool := &GlobSearchTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    "relative/path",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for relative path")
	}
	if !strings.Contains(result.Error, "绝对路径") {
		t.Errorf("expected error about absolute path, got: %s", result.Error)
	}
}

func TestGlobSearchPathIsFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	tool := &GlobSearchTool{}
	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.txt",
		"path":    testFile,
	})
	if err == nil {
		t.Fatal("expected error when path is a file instead of directory")
	}
	toolErr, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected *ToolError, got %T: %v", err, err)
	}
	if !strings.Contains(toolErr.Message, "不是目录") {
		t.Errorf("error message should mention not a directory, got: %s", toolErr.Message)
	}
}
