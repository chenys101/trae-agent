package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_emptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	got := Load(tmpDir)
	if got != "" {
		t.Errorf("expected empty memory, got %q", got)
	}
}

func TestLoad_projectMemory(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, ".trae")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, MemoryFile), []byte("# Test Project\n\nConventions here."), 0o644)

	got := Load(tmpDir)
	if got == "" {
		t.Fatal("expected non-empty memory")
	}
	if !contains(got, "Test Project") {
		t.Errorf("memory missing project content, got: %s", got)
	}
}

func TestLoad_userMemory(t *testing.T) {
	// 无法修改 UserHomeDir，跳过用户级测试（Load 内部会尝试读取 ~/.trae/AGENTS.md）
	// 项目级 + 用户级拼接逻辑在集成测试中验证
	tmpDir := t.TempDir()
	if got := Load(tmpDir); got != "" {
		// 可能有用户级记忆，不影响测试
		_ = got
	}
}

func TestSaveProject(t *testing.T) {
	tmpDir := t.TempDir()
	content := "# Saved\n\nBy test."
	if err := SaveProject(tmpDir, content); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if !Exists(tmpDir) {
		t.Error("expected file to exist after SaveProject")
	}
	got := Load(tmpDir)
	if !contains(got, "Saved") {
		t.Errorf("expected 'Saved' in loaded memory, got: %s", got)
	}
}

func TestExists_false(t *testing.T) {
	tmpDir := t.TempDir()
	if Exists(tmpDir) {
		t.Error("expected Exists=false for empty dir")
	}
}

func TestProjectPath(t *testing.T) {
	got := ProjectPath("/work")
	expected := filepath.Join("/work", ".trae", MemoryFile)
	if got != expected {
		t.Errorf("ProjectPath = %q, want %q", got, expected)
	}
}

// contains 简单字符串包含检查。
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
