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
