package cli

import (
	"context"
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

func TestInterruptHandler_startStop(t *testing.T) {
	h := NewInterruptHandler()
	ctx, cancel := context.WithCancel(context.Background())
	stop := h.Start(ctx)
	cancel()
	stop()
}
