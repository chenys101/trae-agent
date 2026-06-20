package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEdit_strReplace(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("foo bar baz"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]string{
		"file_path":  path,
		"old_string": "bar",
		"new_string": "QUX",
	})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "foo QUX baz" {
		t.Errorf("content = %q, want 'foo QUX baz'", data)
	}
}

func TestEdit_notUnique(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("a a a"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]string{
		"file_path":  path,
		"old_string": "a",
		"new_string": "b",
	})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for non-unique match")
	}
}

func TestEdit_notFound(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("hello"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]string{
		"file_path":  path,
		"old_string": "world",
		"new_string": "x",
	})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for not found")
	}
}

func TestEdit_replaceAll(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("a a a"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]any{
		"file_path":   path,
		"old_string":  "a",
		"new_string":  "b",
		"replace_all": true,
	})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "b b b" {
		t.Errorf("content = %q, want 'b b b'", data)
	}
}

func TestEdit_replaceAll_oldEqualsNew(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("a a a"), 0o644)

	r := NewEdit()
	// old_string == new_string，应成功（不是 "not found"）
	args, _ := json.Marshal(map[string]any{
		"file_path":   path,
		"old_string":  "a",
		"new_string":  "a",
		"replace_all": true,
	})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Errorf("old==new should not be error: %s", res.Content)
	}
}

func TestEdit_replaceAll_notFound(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("hello"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]any{
		"file_path":   path,
		"old_string":  "world",
		"new_string":  "x",
		"replace_all": true,
	})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for not found with replace_all")
	}
}
