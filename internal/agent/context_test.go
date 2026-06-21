package agent

import (
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestEstimateTokens_empty(t *testing.T) {
	if got := EstimateTokens(nil); got != 0 {
		t.Errorf("empty messages should be 0 tokens, got %d", got)
	}
}

func TestEstimateTokens_singleMessage(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "hello world"},
	}
	got := EstimateTokens(msgs)
	// "hello world" = 11 chars / 3 = 3, + 4 overhead = 7
	if got < 3 || got > 20 {
		t.Errorf("expected 3-20 tokens, got %d", got)
	}
}

func TestEstimateTokens_withToolCalls(t *testing.T) {
	msgs := []llm.Message{
		{
			Role:    llm.RoleAssistant,
			Content: "let me read the file",
			ToolCalls: []llm.ToolCall{
				{ID: "t1", Name: "read", Args: `{"file_path":"/tmp/a.go"}`},
			},
		},
	}
	got := EstimateTokens(msgs)
	if got < 10 {
		t.Errorf("expected >=10 tokens with tool calls, got %d", got)
	}
}

func TestContextManager_ShouldCompact(t *testing.T) {
	cm := NewContextManager(WithMaxTokens(100))
	// 构造超过 100 token 的消息
	msgs := make([]llm.Message, 50)
	for i := range msgs {
		msgs[i] = llm.Message{Role: llm.RoleUser, Content: "this is a long enough message to exceed threshold"}
	}
	if !cm.ShouldCompact(msgs) {
		t.Error("should compact when over threshold")
	}
}

func TestContextManager_ShouldCompact_underThreshold(t *testing.T) {
	cm := NewContextManager(WithMaxTokens(10000))
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "hi"}}
	if cm.ShouldCompact(msgs) {
		t.Error("should not compact under threshold")
	}
}

func TestContextManager_defaults(t *testing.T) {
	cm := NewContextManager()
	if cm.MaxTokens() != 100000 {
		t.Errorf("default maxTokens = %d, want 100000", cm.MaxTokens())
	}
	if cm.CompactKeep() != 6 {
		t.Errorf("default compactKeep = %d, want 6", cm.CompactKeep())
	}
}
