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

func TestGrep_pathNotExist(t *testing.T) {
	r := NewGrep()
	args, _ := json.Marshal(map[string]string{
		"pattern": "foo",
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

func TestGrep_longLine(t *testing.T) {
	tmp := t.TempDir()
	// 构造 > 64KB 的单行，验证不被 scanner 默认 64KB 上限静默丢弃
	// grep 实现已将 scanner buffer 扩到 1MB，该行应能被匹配
	longLine := strings.Repeat("a", 70000) + "UNIQUE_MARKER"
	os.WriteFile(filepath.Join(tmp, "long.txt"), []byte(longLine), 0o644)
	r := NewGrep()
	args, _ := json.Marshal(map[string]string{
		"pattern": "UNIQUE_MARKER",
		"path":    tmp,
	})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "UNIQUE_MARKER") {
		t.Errorf("long line was dropped, content: %s", res.Content)
	}
}
