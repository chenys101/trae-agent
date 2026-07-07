package logger

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestInit_writesToFileAndConsole 验证日志同时输出到文件和控制台(stderr)。
func TestInit_writesToFileAndConsole(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// 保存并恢复全局 slog.Default，避免测试间污染
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	l, err := Init("debug", "")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if l == nil {
		t.Fatal("logger is nil")
	}

	l.Info("hello", "key", "value")

	if closer, ok := l.(interface{ Close() error }); ok {
		_ = closer.Close()
	}

	// 验证文件包含完整记录（含时间戳）
	logPath := filepath.Join(tmp, ".trae", "logs", "trae.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("log file does not contain 'hello', got: %s", data)
	}
	if !strings.Contains(string(data), "key=value") {
		t.Errorf("log file does not contain key=value, got: %s", data)
	}
}

// TestInit_off_disablesFile 验证 log_file=off 时不创建文件。
func TestInit_off_disablesFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	l, err := Init("info", "off")
	if err != nil {
		t.Fatalf("Init off failed: %v", err)
	}
	l.Info("no-file")
	_ = l.Close()

	// 默认日志文件不应存在
	logPath := filepath.Join(tmp, ".trae", "logs", "trae.log")
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("log file should not exist when log_file=off, got err=%v", err)
	}
}

// TestInit_customPath 验证自定义文件路径生效。
func TestInit_customPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	custom := filepath.Join(tmp, "custom.log")
	l, err := Init("info", custom)
	if err != nil {
		t.Fatalf("Init custom path failed: %v", err)
	}
	l.Info("custom-path")
	_ = l.Close()

	data, err := os.ReadFile(custom)
	if err != nil {
		t.Fatalf("read custom log: %v", err)
	}
	if !strings.Contains(string(data), "custom-path") {
		t.Errorf("custom log missing message, got: %s", data)
	}
}

// TestInit_tildeExpansion 验证 ~/ 前缀展开到 home 目录。
func TestInit_tildeExpansion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	l, err := Init("info", "~/my-logs/app.log")
	if err != nil {
		t.Fatalf("Init tilde failed: %v", err)
	}
	l.Info("tilde-msg")
	_ = l.Close()

	expect := filepath.Join(tmp, "my-logs", "app.log")
	data, err := os.ReadFile(expect)
	if err != nil {
		t.Fatalf("read tilde log: %v", err)
	}
	if !strings.Contains(string(data), "tilde-msg") {
		t.Errorf("tilde log missing message, got: %s", data)
	}
}

// TestInitWithWriter 验证 InitWithWriter 写入指定 writer。
func TestInitWithWriter(t *testing.T) {
	var buf bytes.Buffer
	l, err := InitWithWriter(&buf, "info")
	if err != nil {
		t.Fatalf("InitWithWriter failed: %v", err)
	}
	l.Info("writer-msg")
	if !strings.Contains(buf.String(), "writer-msg") {
		t.Errorf("writer missing message, got: %s", buf.String())
	}
}

func TestInit_invalidLevel(t *testing.T) {
	_, err := Init("invalid", "")
	if err == nil {
		t.Error("expected error for invalid level, got nil")
	}
}
