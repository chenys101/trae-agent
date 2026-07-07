package agent

import "testing"

func TestThinkingBudget(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"just a normal request", 0},
		{"think about this", 4096},
		{"think hard about this", 6144},
		{"think harder about this", 8192},
		{"ultrathink this problem", 12288},
		{"THINK about uppercase", 4096},    // 大小写不敏感
		{"UltraThink mixed case", 12288},   // 混合大小写
		{"please think harder now", 8192},  // 中间出现
		{"I need to think about it", 4096}, // 短语内匹配
		{"no special keyword here", 0},
	}
	for _, tt := range tests {
		got := ThinkingBudget(tt.input)
		if got != tt.want {
			t.Errorf("ThinkingBudget(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// 验证 think 关键词优先级：ultrathink > think harder > think hard > think
func TestThinkingBudget_priority(t *testing.T) {
	// "ultrathink" 包含 "think"，应返回 ultrathink 的预算
	got := ThinkingBudget("ultrathink and think hard")
	if got != 12288 {
		t.Errorf("expected ultrathink budget (12288), got %d", got)
	}
}

func TestBuildSystemPrompt_empty(t *testing.T) {
	got := BuildSystemPrompt("")
	if got != SystemPrompt() {
		t.Error("expected base SystemPrompt when memory is empty")
	}
}

func TestBuildSystemPrompt_withMemory(t *testing.T) {
	got := BuildSystemPrompt("# My Project\n\nConventions")
	if got == SystemPrompt() {
		t.Error("expected modified prompt when memory is provided")
	}
	if !contains(got, "My Project") {
		t.Error("expected memory content in prompt")
	}
	if !contains(got, "Project Context") {
		t.Error("expected 'Project Context' section")
	}
}

func TestPlanModePrefix(t *testing.T) {
	if PlanModePrefix == "" {
		t.Error("expected non-empty PlanModePrefix")
	}
	if !contains(PlanModePrefix, "Plan Mode") {
		t.Error("expected 'Plan Mode' in prefix")
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
