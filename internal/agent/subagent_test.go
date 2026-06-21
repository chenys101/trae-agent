package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bytedance/trae-agent/internal/tool"
)

func TestSubagentType_known(t *testing.T) {
	cases := []struct {
		name      string
		wantTools []string
	}{
		{"search", []string{"read", "glob", "grep"}},
		{"general-purpose", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, ok := GetSubagentType(c.name)
			if !ok {
				t.Fatalf("GetSubagentType(%q) not found", c.name)
			}
			if st.Name != c.name {
				t.Errorf("Name = %q, want %q", st.Name, c.name)
			}
			if st.Description == "" {
				t.Error("Description is empty")
			}
		})
	}
}

func TestSubagentType_unknown(t *testing.T) {
	_, ok := GetSubagentType("nonexistent")
	if ok {
		t.Error("expected false for unknown subagent type")
	}
}

func TestSubagentType_restrictedRegistry_search(t *testing.T) {
	allTools := []tool.Tool{
		&mockTool{name: "read"},
		&mockTool{name: "glob"},
		&mockTool{name: "grep"},
		&mockTool{name: "bash"},
		&mockTool{name: "write"},
		&mockTool{name: "edit"},
	}
	st, _ := GetSubagentType("search")
	reg := st.RestrictedRegistry(tool.NewRegistry(allTools...))
	names := toolNames(reg)
	for _, n := range names {
		if n != "read" && n != "glob" && n != "grep" {
			t.Errorf("search subagent should not have tool %q", n)
		}
	}
	if len(names) != 3 {
		t.Errorf("search subagent should have 3 tools, got %d: %v", len(names), names)
	}
}

func TestSubagentType_restrictedRegistry_general(t *testing.T) {
	allTools := []tool.Tool{
		&mockTool{name: "read"},
		&mockTool{name: "bash"},
	}
	st, _ := GetSubagentType("general-purpose")
	reg := st.RestrictedRegistry(tool.NewRegistry(allTools...))
	if len(reg.List()) != 2 {
		t.Errorf("general-purpose should keep all tools, got %d", len(reg.List()))
	}
}

// mockTool 用于测试的 mock 工具
type mockTool struct {
	name string
}

func (m *mockTool) Name() string            { return m.name }
func (m *mockTool) Description() string     { return "mock " + m.name }
func (m *mockTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (m *mockTool) Run(ctx context.Context, args json.RawMessage) tool.Result {
	return tool.Result{Content: "mock"}
}

func toolNames(r *tool.Registry) []string {
	var names []string
	for _, t := range r.List() {
		names = append(names, t.Name())
	}
	return names
}
