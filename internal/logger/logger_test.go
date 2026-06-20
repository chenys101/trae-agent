package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_writesToFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// 保存并恢复全局 slog.Default，避免测试间污染
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	l, err := Init("debug")
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

func TestInit_invalidLevel(t *testing.T) {
	_, err := Init("invalid")
	if err == nil {
		t.Error("expected error for invalid level, got nil")
	}
}
