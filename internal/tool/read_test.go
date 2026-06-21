package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRead_fullFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

	r := NewRead()
	args, _ := json.Marshal(map[string]string{"file_path": path})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "line1") {
		t.Errorf("missing line1: %s", res.Content)
	}
	// 应含行号前缀
	if !strings.Contains(res.Content, "1→") || !strings.Contains(res.Content, "2→") {
		t.Errorf("missing line numbers: %s", res.Content)
	}
}

func TestRead_withOffsetLimit(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0o644)

	r := NewRead()
	args, _ := json.Marshal(map[string]any{"file_path": path, "offset": 2, "limit": 2})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "b") || !strings.Contains(res.Content, "c") {
		t.Errorf("missing b/c: %s", res.Content)
	}
	if strings.Contains(res.Content, "a") && strings.Contains(res.Content, "d") {
		t.Errorf("should not contain a/d: %s", res.Content)
	}
}

func TestRead_fileNotFound(t *testing.T) {
	r := NewRead()
	args, _ := json.Marshal(map[string]string{"file_path": "/nonexistent"})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for missing file")
	}
}

func TestRead_missingArg(t *testing.T) {
	r := NewRead()
	res := r.Run(context.Background(), json.RawMessage(`{}`))
	if !res.IsError {
		t.Error("expected error for missing file_path")
	}
}

func TestRead_emptyFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "empty.txt")
	os.WriteFile(path, []byte{}, 0o644)

	r := NewRead()
	args, _ := json.Marshal(map[string]string{"file_path": path})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Content != "" {
		t.Errorf("empty file should return empty content, got: %q", res.Content)
	}
}

func TestRead_offsetBeyondFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("a\nb\n"), 0o644)

	r := NewRead()
	args, _ := json.Marshal(map[string]any{"file_path": path, "offset": 100})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Content != "" {
		t.Errorf("offset beyond file should return empty, got: %q", res.Content)
	}
}

// TestRead_preservesTrailingEmptyLines 验证 TrimRight bug 修复：
// 旧实现用 strings.TrimRight(s, "\n") 会吞掉所有尾部换行，
// "a\n\n" 被压缩为 ["a"]，丢失末尾空行。
// 修复后用 TrimSuffix 只移除一个尾部换行，保留空行。
func TestRead_preservesTrailingEmptyLines(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	// 3 行：a, b, 空行
	os.WriteFile(path, []byte("a\nb\n\n"), 0o644)

	r := NewRead()
	args, _ := json.Marshal(map[string]string{"file_path": path})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// 应有 3 行（a, b, 空行），不是 2 行
	lines := strings.Split(strings.TrimSuffix(res.Content, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines (a, b, empty), got %d: %q", len(lines), res.Content)
	}
}

// TestRead_fileTooLarge 验证大文件被拒绝，避免 OOM。
func TestRead_fileTooLarge(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "big.txt")
	// 写入超过 maxFileSize 的文件
	os.WriteFile(path, make([]byte, maxFileSize+1), 0o644)

	r := NewRead()
	args, _ := json.Marshal(map[string]string{"file_path": path})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for oversized file")
	}
	if !strings.Contains(res.Content, "too large") {
		t.Errorf("error should mention 'too large': %s", res.Content)
	}
}
