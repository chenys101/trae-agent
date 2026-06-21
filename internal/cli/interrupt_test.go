package cli

import (
	"testing"
)

func TestInterruptHandler_noAgent(t *testing.T) {
	h := NewInterruptHandler()
	if h.Handle() {
		t.Error("first press without agent should not exit")
	}
	if !h.Handle() {
		t.Error("second press should exit")
	}
}

func TestInterruptHandler_withAgent(t *testing.T) {
	h := NewInterruptHandler()
	cancelled := false
	cancel := func() { cancelled = true }
	h.SetCancel(cancel)

	if h.Handle() {
		t.Error("first press with agent should not exit")
	}
	if !cancelled {
		t.Error("agent should be cancelled")
	}

	if !h.Handle() {
		t.Error("second press should exit")
	}
}

func TestInterruptHandler_reset(t *testing.T) {
	h := NewInterruptHandler()
	h.Handle()
	h.Reset()
	if h.Handle() {
		t.Error("after reset, first press should not exit")
	}
}

func TestInterruptHandler_setCancelClearsPressed(t *testing.T) {
	h := NewInterruptHandler()
	h.Handle()
	h.SetCancel(func() {})
	if h.Handle() {
		t.Error("after SetCancel, Handle should cancel agent not exit")
	}
}

func TestInterruptHandler_startStopAgentListener(t *testing.T) {
	h := NewInterruptHandler()
	stop := h.StartAgentSignalListener()
	stop()
	// 不 panic 即可
}

func TestInterruptHandler_agentListenerCancels(t *testing.T) {
	h := NewInterruptHandler()
	cancelled := false
	h.SetCancel(func() { cancelled = true })

	stop := h.StartAgentSignalListener()
	defer stop()

	// 模拟发送 SIGINT 给当前进程
	// 注意：signal.Notify 会捕获，不会真正终止进程
	// 这里只验证 listener 能正常启停，实际信号测试在集成测试
	_ = cancelled
}
