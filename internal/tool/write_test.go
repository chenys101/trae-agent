package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWrite_createFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "new.txt")
	r := NewWrite()
	args, _ := json.Marshal(map[string]string{"file_path": path, "content": "hello"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "hello" {
		t.Errorf("file content = %q, want hello", data)
	}
}

func TestWrite_overwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("old"), 0o644)

	r := NewWrite()
	args, _ := json.Marshal(map[string]string{"file_path": path, "content": "new"})
	r.Run(context.Background(), args)
	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("content = %q, want new", data)
	}
}

func TestWrite_createParentDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sub", "dir", "f.txt")
	r := NewWrite()
	args, _ := json.Marshal(map[string]string{"file_path": path, "content": "x"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestWrite_missingArg(t *testing.T) {
	r := NewWrite()
	res := r.Run(context.Background(), json.RawMessage(`{}`))
	if !res.IsError {
		t.Error("expected error for missing args")
	}
}
