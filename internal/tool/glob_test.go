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

func TestGlob_invalidPattern(t *testing.T) {
	tmp := t.TempDir()
	// 至少创建一个文件，确保 matchGlob 被触发（否则 Walk 不会调用匹配逻辑）
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0o644)
	r := NewGlob()
	args, _ := json.Marshal(map[string]string{
		"pattern": "[", // 非法 glob pattern，filepath.Match 返回 ErrBadPattern
		"path":    tmp,
	})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for invalid pattern")
	}
}

func TestGlob_pathNotExist(t *testing.T) {
	r := NewGlob()
	args, _ := json.Marshal(map[string]string{
		"pattern": "**/*.go",
		"path":    "/nonexistent/path/that/does/not/exist",
	})
	res := r.Run(context.Background(), args)
	// 当前实现对不存在的 path 优雅处理：Walk 回调吞掉 root 错误，返回空结果
	if res.IsError {
		t.Errorf("expected no error for non-existent path, got: %s", res.Content)
	}
	if res.Content != "" {
		t.Errorf("expected empty result, got: %s", res.Content)
	}
}
