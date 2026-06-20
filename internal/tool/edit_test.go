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
