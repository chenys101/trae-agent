package cli

import (
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/tool"
)

// newCompletionTestREPL 构造用于补全测试的 REPL。
func newCompletionTestREPL(t *testing.T) *REPL {
	t.Helper()
	a := agent.New(&cliMockProvider{}, tool.NewRegistry(), agent.WithModel("test-model"))
	return &REPL{
		agent:    a,
		commands: NewCommandRegistry(),
	}
}

// TestSuggestCommands_slashOnly 验证输入 "/" 时列出所有命令。
func TestSuggestCommands_slashOnly(t *testing.T) {
	r := newCompletionTestREPL(t)
	got := r.suggestCommands("/")
	// 应包含核心命令
	for _, want := range []string{"/help", "/exit", "/model", "/clear"} {
		if !strings.Contains(got, want) {
			t.Errorf("suggestCommands(\"/\") missing %q, got:\n%s", want, got)
		}
	}
}

// TestSuggestCommands_emptyInput 验证空输入也列出所有命令。
func TestSuggestCommands_emptyInput(t *testing.T) {
	r := newCompletionTestREPL(t)
	got := r.suggestCommands("")
	if !strings.Contains(got, "/help") {
		t.Errorf("suggestCommands(\"\") should list commands, got:\n%s", got)
	}
}

// TestSuggestCommands_prefixMatch 验证前缀匹配。
func TestSuggestCommands_prefixMatch(t *testing.T) {
	r := newCompletionTestREPL(t)
	// /s 前缀应匹配 /status, /sessions, /show-config(如有)
	got := r.suggestCommands("/s")
	for _, want := range []string{"/status", "/sessions"} {
		if !strings.Contains(got, want) {
			t.Errorf("suggestCommands(\"/s\") missing %q, got:\n%s", want, got)
		}
	}
	// 不应包含 /help, /exit
	if strings.Contains(got, "/help") {
		t.Errorf("suggestCommands(\"/s\") should not contain /help")
	}
}

// TestSuggestCommands_typoSuggestion 验证拼写错误的近似建议。
func TestSuggestCommands_typoSuggestion(t *testing.T) {
	r := newCompletionTestREPL(t)
	// /hlep 应建议 /help（编辑距离 2）
	got := r.suggestCommands("/hlep")
	if !strings.Contains(got, "/help") {
		t.Errorf("suggestCommands(\"/hlep\") should suggest /help, got:\n%s", got)
	}
	// 应包含 "Did you mean" 提示
	if !strings.Contains(got, "Did you mean") {
		t.Errorf("should contain 'Did you mean', got:\n%s", got)
	}
}

// TestSuggestCommands_noMatch 验证完全无匹配时返回空。
func TestSuggestCommands_noMatch(t *testing.T) {
	r := newCompletionTestREPL(t)
	got := r.suggestCommands("/zzzzzz")
	if got != "" {
		t.Errorf("suggestCommands(\"/zzzzzz\") should return empty, got: %s", got)
	}
}

// TestHandleCommand_slashAlone 验证 "/" 单独输入显示命令列表。
func TestHandleCommand_slashAlone(t *testing.T) {
	r := newCompletionTestREPL(t)
	// handleCommand 通过 println 输出到 stdout，
	// 这里用 suggestCommands 验证逻辑，handleCommand 的 "/" 分支调用它
	got := r.suggestCommands("/")
	if !strings.Contains(got, "/help") {
		t.Errorf("\"/\" should list all commands, got:\n%s", got)
	}
}

// TestLevenshtein 验证编辑距离计算。
func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"/hlep", "/help", 2}, // 交换 e 和 l
		{"/cat", "/car", 1},
		{"/status", "/stats", 1}, // 删除 u 即可
	}
	for _, c := range cases {
		got := levenshtein(c.a, c.b)
		if got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestCompleter_includesCustomCommands 验证补全器包含自定义命令。
func TestCompleter_includesCustomCommands(t *testing.T) {
	// 用默认 registry（含内置命令）
	r := NewCommandRegistry()
	completer := r.Completer()
	if completer == nil {
		t.Fatal("Completer() returned nil")
	}
	// 内置命令应在补全器中
	// readline.PrefixCompleter 内部结构不暴露遍历接口，
	// 这里仅验证不 panic 且返回非 nil
}

// TestCommand_helpSpecificCommand 验证 /help <command> 显示详细用法。
func TestCommand_helpSpecificCommand(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/help")
	repl := &REPL{commands: r, agent: newTestAgent(t, "test")}

	// /help /model：显示 /model 的详细用法
	result := cmd.Handler(repl, []string{"/model"})
	if !strings.Contains(result.Message, "model") {
		t.Errorf("expected 'model' in result, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "usage:") {
		t.Errorf("expected 'usage:' in result, got: %s", result.Message)
	}

	// /help model（无 / 前缀，自动补全）
	result = cmd.Handler(repl, []string{"model"})
	if !strings.Contains(result.Message, "/model") {
		t.Errorf("expected '/model' in result, got: %s", result.Message)
	}
}

// TestCommand_helpAll 验证 /help 无参数显示所有命令和 Tips。
func TestCommand_helpAll(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/help")
	repl := &REPL{commands: r, agent: newTestAgent(t, "test")}

	result := cmd.Handler(repl, nil)
	// 应包含 Tips
	if !strings.Contains(result.Message, "Tips") {
		t.Errorf("expected 'Tips' section, got: %s", result.Message)
	}
	// 应包含 /exit
	if !strings.Contains(result.Message, "/exit") {
		t.Errorf("expected '/exit' in list, got: %s", result.Message)
	}
}
