package cli

import (
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
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
