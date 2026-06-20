package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrep_match(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("foo\nbar\nbaz\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("qux\nfoo\n"), 0o644)

	r := NewGrep()
	args, _ := json.Marshal(map[string]string{"pattern": "foo", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.txt") {
		t.Errorf("missing a.txt: %s", res.Content)
	}
	if !strings.Contains(res.Content, "b.txt") {
		t.Errorf("missing b.txt: %s", res.Content)
	}
}

func TestGrep_regex(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("test123\ntest\n"), 0o644)

	r := NewGrep()
	args, _ := json.Marshal(map[string]string{"pattern": "test\\d+", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "test123") {
		t.Errorf("missing test123: %s", res.Content)
	}
	if strings.Contains(res.Content, "\ntest\n") {
		t.Errorf("should not match plain 'test': %s", res.Content)
	}
}

func TestGrep_noMatch(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0o644)

	r := NewGrep()
	args, _ := json.Marshal(map[string]string{"pattern": "world", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Errorf("no match should not be error: %s", res.Content)
	}
}
