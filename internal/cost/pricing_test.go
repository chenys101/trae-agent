package cost

import (
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestEstimate_claude35Sonnet(t *testing.T) {
	cost, ok := Estimate("claude-3-5-sonnet-20241022", llm.Usage{InputTokens: 1000000, OutputTokens: 1000000})
	if !ok {
		t.Fatal("expected match")
	}
	if cost < 17.9 || cost > 18.1 {
		t.Errorf("expected ~$18.0, got %.4f", cost)
	}
}

func TestEstimate_gpt4o(t *testing.T) {
	cost, ok := Estimate("gpt-4o-2024-08-06", llm.Usage{InputTokens: 1000000, OutputTokens: 500000})
	if !ok {
		t.Fatal("expected match")
	}
	if cost < 7.4 || cost > 7.6 {
		t.Errorf("expected ~$7.5, got %.4f", cost)
	}
}

func TestEstimate_caseInsensitive(t *testing.T) {
	cost1, ok1 := Estimate("Claude-3-5-Sonnet", llm.Usage{InputTokens: 100, OutputTokens: 100})
	cost2, ok2 := Estimate("claude-3-5-sonnet", llm.Usage{InputTokens: 100, OutputTokens: 100})
	if !ok1 || !ok2 {
		t.Fatal("expected both to match")
	}
	if cost1 != cost2 {
		t.Errorf("case insensitive mismatch: %v vs %v", cost1, cost2)
	}
}

func TestEstimate_unknownModel(t *testing.T) {
	_, ok := Estimate("some-unknown-model", llm.Usage{})
	if ok {
		t.Error("expected no match for unknown model")
	}
}

func TestEstimate_emptyModel(t *testing.T) {
	_, ok := Estimate("", llm.Usage{})
	if ok {
		t.Error("expected no match for empty model")
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		cost float64
		want string
	}{
		{0.001, "$0.0010"},
		{0.005, "$0.0050"},
		{0.01, "$0.01"},
		{0.15, "$0.15"},
		{1.5, "$1.50"},
		{18.0, "$18.00"},
	}
	for _, tt := range tests {
		got := FormatCost(tt.cost)
		if got != tt.want {
			t.Errorf("FormatCost(%.4f) = %q, want %q", tt.cost, got, tt.want)
		}
	}
}

func TestEstimate_prefixSpecificity(t *testing.T) {
	cost, ok := Estimate("gpt-4o-mini", llm.Usage{InputTokens: 1000000, OutputTokens: 0})
	if !ok {
		t.Fatal("expected match")
	}
	if cost < 0.14 || cost > 0.16 {
		t.Errorf("expected ~$0.15 for gpt-4o-mini, got %.4f", cost)
	}
}

func TestEstimate_zeroUsage(t *testing.T) {
	cost, ok := Estimate("gpt-4o", llm.Usage{})
	if !ok {
		t.Fatal("expected match")
	}
	if cost != 0 {
		t.Errorf("expected $0 for zero usage, got %.4f", cost)
	}
	if !strings.HasPrefix(FormatCost(cost), "$") {
		t.Errorf("FormatCost should start with $")
	}
}
