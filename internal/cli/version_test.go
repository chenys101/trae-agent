package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand_output(t *testing.T) {
	// 保存并恢复包级变量，避免测试间污染
	prevVersion, prevCommit := Version, Commit
	t.Cleanup(func() {
		Version, Commit = prevVersion, prevCommit
	})

	Version = "0.1.0"
	Commit = "abc1234"
	var buf bytes.Buffer
	cmd := NewVersionCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.Run(cmd, []string{})
	out := buf.String()
	if !strings.Contains(out, "0.1.0") {
		t.Errorf("output missing version, got: %s", out)
	}
	if !strings.Contains(out, "abc1234") {
		t.Errorf("output missing commit, got: %s", out)
	}
}
