package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCustomCommands_emptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	cmds := LoadCustomCommands(tmpDir)
	if len(cmds) != 0 {
		t.Errorf("expected 0 commands, got %d", len(cmds))
	}
}

func TestLoadCustomCommands_noDir(t *testing.T) {
	cmds := LoadCustomCommands("/nonexistent/path")
	if cmds != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", cmds)
	}
}

func TestLoadCustomCommands_loadsMarkdown(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".trae", "commands")
	os.MkdirAll(cmdDir, 0o755)

	// 创建测试命令文件
	os.WriteFile(filepath.Join(cmdDir, "review.md"), []byte(`---
description: Review code for issues
---

Review the following code for issues:

$ARGUMENTS

Provide specific recommendations for improvements.`), 0o644)

	cmds := LoadCustomCommands(tmpDir)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].name != "/review" {
		t.Errorf("expected name '/review', got %q", cmds[0].name)
	}
	if cmds[0].description != "Review code for issues" {
		t.Errorf("expected description, got %q", cmds[0].description)
	}
}

func TestLoadCustomCommands_multipleSorted(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".trae", "commands")
	os.MkdirAll(cmdDir, 0o755)

	os.WriteFile(filepath.Join(cmdDir, "zebra.md"), []byte("Z body"), 0o644)
	os.WriteFile(filepath.Join(cmdDir, "alpha.md"), []byte("A body"), 0o644)

	cmds := LoadCustomCommands(tmpDir)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
	// 验证按名称排序
	if cmds[0].name != "/alpha" {
		t.Errorf("expected '/alpha' first, got %q", cmds[0].name)
	}
}

func TestLoadCustomCommands_ignoresNonMarkdown(t *testing.T) {
	tmpDir := t.TempDir()
	cmdDir := filepath.Join(tmpDir, ".trae", "commands")
	os.MkdirAll(cmdDir, 0o755)

	os.WriteFile(filepath.Join(cmdDir, "test.txt"), []byte("ignore"), 0o644)
	os.WriteFile(filepath.Join(cmdDir, "valid.md"), []byte("valid"), 0o644)

	cmds := LoadCustomCommands(tmpDir)
	if len(cmds) != 1 {
		t.Errorf("expected 1 command, got %d", len(cmds))
	}
}

func TestRenderBody_argumentsSubstitution(t *testing.T) {
	cc := &CustomCommand{
		name: "test",
		body: "Search for: $ARGUMENTS. First arg: $1. Second: $2.",
	}
	got := cc.renderBody([]string{"foo", "bar"})
	wantContains := []string{"Search for: foo bar.", "First arg: foo.", "Second: bar."}
	for _, w := range wantContains {
		if !contains(got, w) {
			t.Errorf("expected %q in rendered body, got: %s", w, got)
		}
	}
}

func TestRenderBody_noArgs(t *testing.T) {
	cc := &CustomCommand{
		name: "test",
		body: "Run tests with no args. $ARGUMENTS stays empty.",
	}
	got := cc.renderBody(nil)
	// $ARGUMENTS 应被替换为空字符串
	if !contains(got, "stays empty.") {
		t.Errorf("unexpected rendered body: %s", got)
	}
}

func TestParseFrontmatter_noFrontmatter(t *testing.T) {
	desc, body := parseFrontmatter("Just plain markdown.")
	if desc != "" {
		t.Errorf("expected empty desc, got %q", desc)
	}
	if body != "Just plain markdown." {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestParseFrontmatter_withFrontmatter(t *testing.T) {
	content := `---
description: My command
---

Body here.`
	desc, body := parseFrontmatter(content)
	if desc != "My command" {
		t.Errorf("expected 'My command', got %q", desc)
	}
	if body != "Body here." {
		t.Errorf("unexpected body: %q", body)
	}
}

func TestToCommand(t *testing.T) {
	cc := &CustomCommand{
		name:        "/test-cmd",
		description: "Test description",
		body:        "Test prompt body",
	}
	cmd := cc.ToCommand()
	if cmd.Name != "/test-cmd" {
		t.Errorf("name = %q", cmd.Name)
	}
	if cmd.Description != "Test description" {
		t.Errorf("description = %q", cmd.Description)
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
