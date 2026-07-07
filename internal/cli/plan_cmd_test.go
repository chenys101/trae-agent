package cli

import "testing"

func TestPlanCmd_toggle(t *testing.T) {
	repl := &REPL{}
	cmd := NewPlanCmd()

	// 初始状态：planMode = false
	if repl.planMode {
		t.Fatal("expected planMode false initially")
	}

	// 第一次调用：开启
	result := cmd.Handler(repl, nil)
	if !repl.planMode {
		t.Error("expected planMode true after first toggle")
	}
	if result.Exit {
		t.Error("should not exit")
	}

	// 第二次调用：关闭
	result = cmd.Handler(repl, nil)
	if repl.planMode {
		t.Error("expected planMode false after second toggle")
	}
	if result.Exit {
		t.Error("should not exit")
	}
}

func TestPlanCmd_messageContent(t *testing.T) {
	repl := &REPL{}
	cmd := NewPlanCmd()

	// 开启时应提示 ON
	result := cmd.Handler(repl, nil)
	if !contains(result.Message, "ON") {
		t.Errorf("expected 'ON' in message, got: %s", result.Message)
	}

	// 关闭时应提示 OFF
	result = cmd.Handler(repl, nil)
	if !contains(result.Message, "OFF") {
		t.Errorf("expected 'OFF' in message, got: %s", result.Message)
	}
}
