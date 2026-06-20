package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowConfigCommand_output(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	os.MkdirAll(filepath.Dir(userCfg), 0o755)
	os.WriteFile(userCfg, []byte("default_provider: anthropic\nlog_level: debug\n"), 0o644)

	var buf bytes.Buffer
	cmd := NewShowConfigCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	// show-config 用 RunE，测试也用 RunE
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("RunE failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "anthropic") {
		t.Errorf("output missing provider, got: %s", out)
	}
	if !strings.Contains(out, "debug") {
		t.Errorf("output missing log level, got: %s", out)
	}
}
