package cli

import (
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/mcp"
	"github.com/bytedance/trae-agent/internal/permission"
)

func TestCommand_mcp_nilManager(t *testing.T) {
	r := NewCommandRegistry()
	cmd, ok := r.Get("/mcp")
	if !ok {
		t.Fatal("/mcp not found")
	}
	repl := &REPL{commands: r, mcpMgr: nil}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "not configured") {
		t.Errorf("expected 'not configured', got: %s", result.Message)
	}
}

func TestCommand_mcp_noServers(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/mcp")
	repl := &REPL{commands: r, mcpMgr: mcp.NewManager()}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "no MCP servers") {
		t.Errorf("expected 'no MCP servers', got: %s", result.Message)
	}
}

func TestCommand_permissions_nilPolicy(t *testing.T) {
	r := NewCommandRegistry()
	cmd, ok := r.Get("/permissions")
	if !ok {
		t.Fatal("/permissions not found")
	}
	repl := &REPL{commands: r, policy: nil}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "not configured") {
		t.Errorf("expected 'not configured', got: %s", result.Message)
	}
}

func TestCommand_permissions_listBuiltins(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/permissions")
	repl := &REPL{commands: r, policy: permission.NewPolicy()}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "builtin") {
		t.Errorf("expected builtin rules, got: %s", result.Message)
	}
	if !strings.Contains(result.Message, "rm -rf") {
		t.Errorf("expected 'rm -rf', got: %s", result.Message)
	}
}
