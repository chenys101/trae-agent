package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlob_match(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "sub"), 0o755)
	os.WriteFile(filepath.Join(tmp, "a.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(tmp, "sub", "c.go"), []byte("x"), 0o644)

	r := NewGlob()
	args, _ := json.Marshal(map[string]string{"pattern": "**/*.go", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.go") {
		t.Errorf("missing a.go: %s", res.Content)
	}
	if !strings.Contains(res.Content, "c.go") {
		t.Errorf("missing c.go: %s", res.Content)
	}
	if strings.Contains(res.Content, "b.txt") {
		t.Errorf("should not contain b.txt: %s", res.Content)
	}
}

func TestGlob_noMatch(t *testing.T) {
	tmp := t.TempDir()
	r := NewGlob()
	args, _ := json.Marshal(map[string]string{"pattern": "**/*.xyz", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Errorf("no match should not be error: %s", res.Content)
	}
	if res.Content != "" {
		t.Errorf("expected empty, got: %s", res.Content)
	}
}
