package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/session"
	"github.com/bytedance/trae-agent/internal/tool"
)

func TestCommandRegistry_Get(t *testing.T) {
	r := NewCommandRegistry()
	cmd, ok := r.Get("/help")
	if !ok {
		t.Fatal("/help not found")
	}
	if cmd.Name != "/help" {
		t.Errorf("name = %q", cmd.Name)
	}
}

func TestCommandRegistry_Get_notFound(t *testing.T) {
	r := NewCommandRegistry()
	_, ok := r.Get("/nope")
	if ok {
		t.Error("expected not found")
	}
}

func TestCommandRegistry_List_order(t *testing.T) {
	r := NewCommandRegistry()
	list := r.List()
	if len(list) < 5 {
		t.Errorf("expected >=5 commands, got %d", len(list))
	}
	if list[0].Name != "/help" {
		t.Errorf("first command = %q, want /help", list[0].Name)
	}
}

func TestCommand_help(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/help")
	repl := &REPL{commands: r}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "/help") {
		t.Errorf("help output missing /help: %s", result.Message)
	}
	if !strings.Contains(result.Message, "/exit") {
		t.Errorf("help output missing /exit: %s", result.Message)
	}
}

func TestCommand_exit(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/exit")
	result := cmd.Handler(&REPL{commands: r}, nil)
	if !result.Exit {
		t.Error("/exit should return Exit=true")
	}
}

func TestCommand_clear(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/clear")
	repl := &REPL{
		commands: r,
		messages: []llm.Message{{Role: llm.RoleUser, Content: "x"}},
	}
	result := cmd.Handler(repl, nil)
	if len(repl.messages) != 0 {
		t.Error("/clear should empty messages")
	}
	if result.Message != "conversation cleared (previous session saved)" {
		t.Errorf("message = %q", result.Message)
	}
}

func TestCommand_status(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/status")
	repl := &REPL{commands: r}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "messages:") {
		t.Errorf("status missing messages: %s", result.Message)
	}
}

func TestCommand_cost(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/cost")
	repl := &REPL{commands: r}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "total tokens") {
		t.Errorf("cost missing tokens: %s", result.Message)
	}
}

func TestCommandRegistry_completer(t *testing.T) {
	r := NewCommandRegistry()
	c := r.Completer()
	if c == nil {
		t.Fatal("completer is nil")
	}
	if len(c.Children) < 5 {
		t.Errorf("expected >=5 completer children, got %d", len(c.Children))
	}
}

// newTestAgent 构造用于测试的 agent（model=test-model，无工具）。
func newTestAgent(t *testing.T, model string) *agent.Agent {
	t.Helper()
	return agent.New(&cliMockProvider{}, tool.NewRegistry(), agent.WithModel(model))
}

// TestCommand_model 验证 /model 无参数时显示实际模型名，有参数时返回 not supported 错误。
func TestCommand_model(t *testing.T) {
	r := NewCommandRegistry()
	cmd, ok := r.Get("/model")
	if !ok {
		t.Fatal("/model not found")
	}

	repl := &REPL{commands: r, agent: newTestAgent(t, "test-model")}

	// 无参数：显示当前模型
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "test-model") {
		t.Errorf("expected model name in message, got: %s", result.Message)
	}

	// 有参数：返回 not supported 错误
	result = cmd.Handler(repl, []string{"gpt-4"})
	if !strings.Contains(result.Message, "not supported") {
		t.Errorf("expected 'not supported' in message, got: %s", result.Message)
	}
}

// TestCommand_compact_emptyMessages 验证 /compact 在空消息时返回提示。
func TestCommand_compact_emptyMessages(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/compact")
	repl := &REPL{commands: r, messages: nil}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "no messages to compact") {
		t.Errorf("expected empty messages hint, got: %s", result.Message)
	}
}

// TestCommand_sessions_empty 验证 /sessions 在空 store 时返回 no sessions。
func TestCommand_sessions_empty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	store, err := session.NewStoreWithDir(dir)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	r := NewCommandRegistry()
	cmd, _ := r.Get("/sessions")
	repl := &REPL{commands: r, store: store}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "no saved sessions") {
		t.Errorf("expected 'no saved sessions', got: %s", result.Message)
	}
}

// TestCommand_resume_noArgs 验证 /resume 无参数时返回错误提示。
func TestCommand_resume_noArgs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	store, err := session.NewStoreWithDir(dir)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	r := NewCommandRegistry()
	cmd, _ := r.Get("/resume")
	repl := &REPL{commands: r, store: store}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "usage:") {
		t.Errorf("expected usage hint, got: %s", result.Message)
	}
}

// TestCommand_resume_invalidId 验证 /resume 无效 ID 时返回错误。
func TestCommand_resume_invalidId(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	store, err := session.NewStoreWithDir(dir)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	r := NewCommandRegistry()
	cmd, _ := r.Get("/resume")
	repl := &REPL{commands: r, store: store}
	// 使用合法字符但不存在的 ID
	result := cmd.Handler(repl, []string{"nonexistent-id-12345"})
	if !strings.Contains(result.Message, "load session") && !strings.Contains(result.Message, "no such file") {
		t.Errorf("expected load error, got: %s", result.Message)
	}
}
