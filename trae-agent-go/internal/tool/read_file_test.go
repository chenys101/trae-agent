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

func TestReadFileToolName(t *testing.T) {
	tool := &ReadFileTool{}
	if tool.GetName() != "read_file" {
		t.Errorf("expected name 'read_file', got '%s'", tool.GetName())
	}
}

func TestReadFileToolReadFullFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path": tmpFile,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, tmpFile) {
		t.Errorf("output should contain file path, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "(3行)") {
		t.Errorf("output should contain total line count, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "1\tline1") {
		t.Errorf("output should contain line 1, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "2\tline2") {
		t.Errorf("output should contain line 2, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "3\tline3") {
		t.Errorf("output should contain line 3, got: %s", result.Output)
	}
}

func TestReadFileToolWithOffsetAndLimit(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path": tmpFile,
		"offset":    float64(2),
		"limit":     float64(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "(5行)") {
		t.Errorf("output should show total 5 lines, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "2\tline2") {
		t.Errorf("output should contain line 2, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "3\tline3") {
		t.Errorf("output should contain line 3, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "1\tline1") {
		t.Errorf("output should not contain line 1, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "4\tline4") {
		t.Errorf("output should not contain line 4, got: %s", result.Output)
	}
}

func TestReadFileToolFileNotExist(t *testing.T) {
	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "/nonexistent/path/file.txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for nonexistent file")
	}
	if !strings.Contains(result.Error, "文件不存在") {
		t.Errorf("error should mention file not found, got: %s", result.Error)
	}
}

func TestReadFileToolRelativePath(t *testing.T) {
	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path": "relative/path/file.txt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for relative path")
	}
	if !strings.Contains(result.Error, "绝对路径") {
		t.Errorf("error should mention absolute path requirement, got: %s", result.Error)
	}
}

func TestReadFileToolDirectoryPath(t *testing.T) {
	tmpDir := t.TempDir()

	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"file_path": tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for directory path")
	}
	if !strings.Contains(result.Error, "目录") {
		t.Errorf("error should mention directory, got: %s", result.Error)
	}
}

func TestReadFileToolMissingFilePath(t *testing.T) {
	tool := &ReadFileTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing file_path")
	}
	if !strings.Contains(result.Error, "file_path") {
		t.Errorf("error should mention file_path, got: %s", result.Error)
	}
}
