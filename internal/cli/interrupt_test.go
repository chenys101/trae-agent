package cli

import (
	"sync/atomic"
	"syscall"
	"testing"
	"time"
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

// TestInterruptHandler_doublePressTimeWindow 验证双击超过 2 秒窗口则重置。
// 第一次 Handle() 返回 false，等待 > 2 秒，第二次 Handle() 仍返回 false（未触发退出）。
func TestInterruptHandler_doublePressTimeWindow(t *testing.T) {
	h := NewInterruptHandler()

	// 第一次按下：返回 false（不退出）
	if h.Handle() {
		t.Error("first press should not exit")
	}

	// 等待超过双击窗口（doublePressWindow = 2s）
	time.Sleep(doublePressWindow + 100*time.Millisecond)

	// 第二次按下：因超过窗口，pressed 被重置，视为首次按下，仍返回 false
	if h.Handle() {
		t.Error("second press after window expiry should not exit (treated as first press)")
	}
}

// TestInterruptHandler_agentListenerCancels 验证 agent 运行时收到 SIGINT 会调用 cancel。
// 用 syscall.Kill 发送 SIGINT 给当前进程，验证 cancel 被调用。
func TestInterruptHandler_agentListenerCancels(t *testing.T) {
	h := NewInterruptHandler()
	var cancelled int32
	h.SetCancel(func() { atomic.StoreInt32(&cancelled, 1) })

	stop := h.StartAgentSignalListener()
	defer stop()

	// 发送 SIGINT 给当前进程，signal.Notify 会捕获，不会终止进程
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("syscall.Kill failed: %v", err)
	}

	// 等待 goroutine 处理信号（轮询最多 1 秒）
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&cancelled) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if atomic.LoadInt32(&cancelled) != 1 {
		t.Error("expected cancel to be called after SIGINT, but it was not")
	}
}
