package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setTempHome 将 HOME / USERPROFILE 指向临时目录，便于隔离测试。
// 返回 cleanup 函数恢复原值。
func setTempHome(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	old, _ := os.LookupEnv("HOME")
	oldUp, _ := os.LookupEnv("USERPROFILE")
	os.Setenv("HOME", dir)
	os.Setenv("USERPROFILE", dir)
	return dir, func() {
		os.Setenv("HOME", old)
		os.Setenv("USERPROFILE", oldUp)
	}
}

// TestTraeDir_CreatesDir 验证 TraeDir 在主目录下创建 .trae。
func TestTraeDir_CreatesDir(t *testing.T) {
	home, cleanup := setTempHome(t)
	defer cleanup()

	got, err := TraeDir()
	if err != nil {
		t.Fatalf("TraeDir: %v", err)
	}
	want := filepath.Join(home, ".trae")
	if got != want {
		t.Fatalf("TraeDir = %q, want %q", got, want)
	}
	if info, err := os.Stat(got); err != nil || !info.IsDir() {
		t.Fatalf(".trae dir not created: %v", err)
	}
}

// TestUnderTrae_CreatesParent 验证多层子路径的父目录被创建。
func TestUnderTrae_CreatesParent(t *testing.T) {
	_, cleanup := setTempHome(t)
	defer cleanup()

	got, err := UnderTrae("sessions", "abc", "state.json")
	if err != nil {
		t.Fatalf("UnderTrae: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join("sessions", "abc", "state.json")) {
		t.Fatalf("unexpected path: %q", got)
	}
	// 父目录应当已创建（state.json 文件父目录）
	parent := filepath.Dir(got)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		t.Fatalf("parent dir not created: %v", err)
	}
}

// TestUserConfigPath 验证用户级配置文件路径正确。
func TestUserConfigPath(t *testing.T) {
	home, cleanup := setTempHome(t)
	defer cleanup()

	got, err := UserConfigPath()
	if err != nil {
		t.Fatalf("UserConfigPath: %v", err)
	}
	want := filepath.Join(home, ".trae", "config.yaml")
	if got != want {
		t.Fatalf("UserConfigPath = %q, want %q", got, want)
	}
}

// TestProjectConfigPath 验证项目级配置路径不依赖 home，纯拼接。
func TestProjectConfigPath(t *testing.T) {
	got := ProjectConfigPath("/proj")
	want := filepath.Join("/proj", ".trae", "config.yaml")
	if got != want {
		t.Fatalf("ProjectConfigPath = %q, want %q", got, want)
	}
}
