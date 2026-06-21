package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
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

func TestSubagentRunner_executesAndReturnsText(t *testing.T) {
	provider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.TextDelta{Content: "subagent result"},
				llm.Done{},
			},
		},
	}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	st, _ := GetSubagentType("search")
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	result, err := runner.Run(context.Background(), st, "find all TODO comments")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result != "subagent result" {
		t.Errorf("result = %q, want 'subagent result'", result)
	}
}

func TestSubagentRunner_toolLoopReturnsFinalText(t *testing.T) {
	provider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "tc1", Name: "read", ArgsDelta: `{"file_path":"/tmp/x"}`},
				llm.Done{Usage: llm.Usage{InputTokens: 5, OutputTokens: 5}},
			},
			{
				llm.TextDelta{Content: "found 2 TODOs"},
				llm.Done{Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}},
			},
		},
	}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	st, _ := GetSubagentType("search")
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	result, err := runner.Run(context.Background(), st, "find TODOs")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result != "found 2 TODOs" {
		t.Errorf("result = %q, want 'found 2 TODOs'", result)
	}
}

func TestSubagentRunner_contextCancel(t *testing.T) {
	provider := &mockProvider{block: true}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	st, _ := GetSubagentType("search")
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runner.Run(ctx, st, "test")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestSubagentRunner_unknownType(t *testing.T) {
	provider := &mockProvider{}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	_, err := runner.Run(context.Background(), SubagentType{Name: "unknown"}, "test")
	if err == nil {
		t.Error("expected error for unknown subagent type")
	}
}

func TestSubagentRunner_implementsTaskRunner(t *testing.T) {
	provider := &mockProvider{}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	var _ interface {
		RunSubagent(ctx context.Context, subagentType, description, prompt string) (string, error)
	} = runner
}

func TestSubagentRunner_runSubagent(t *testing.T) {
	provider := &mockProvider{
		scripts: [][]llm.StreamEvent{
			{
				llm.TextDelta{Content: "task complete"},
				llm.Done{},
			},
		},
	}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	result, err := runner.RunSubagent(
		context.Background(),
		"search",
		"find TODOs",
		"find all TODO comments in the codebase",
	)
	if err != nil {
		t.Fatalf("RunSubagent failed: %v", err)
	}
	if result != "task complete" {
		t.Errorf("result = %q, want 'task complete'", result)
	}
}
