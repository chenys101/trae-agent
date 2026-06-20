package cli

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// InterruptHandler 管理 Ctrl+C 信号。
// 第一次 Ctrl+C 中断当前 agent 运行，第二次退出 REPL。
type InterruptHandler struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	pressed bool
}

func NewInterruptHandler() *InterruptHandler {
	return &InterruptHandler{}
}

// SetCancel 设置当前 agent 运行的 cancel 函数。
func (h *InterruptHandler) SetCancel(cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancel = cancel
	h.pressed = false
}

// Handle 处理一次 Ctrl+C 信号。
// 返回 true 表示应退出 REPL，false 表示仅中断当前 agent。
func (h *InterruptHandler) Handle() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
		h.pressed = true
		return false
	}

	if h.pressed {
		return true
	}
	h.pressed = true
	return false
}

// Reset 重置 pressed 状态。
func (h *InterruptHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pressed = false
}

// Start 启动信号监听，返回停止函数。
func (h *InterruptHandler) Start(ctx context.Context) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				h.Handle()
			}
		}
	}()
	return func() {
		signal.Stop(sigCh)
	}
}
