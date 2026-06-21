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
// agent 开始时调 SetCancel(cancel)，结束时调 SetCancel(nil)。
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

// Reset 重置 pressed 状态（用户开始输入新内容时调）。
func (h *InterruptHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pressed = false
}

// StartAgentSignalListener 在 agent 运行期间监听 SIGINT，调 cancel 中断 agent。
// 返回 stop 函数，agent 结束时必须调用。
// 仅在 agent 运行时启用，避免与 readline 的信号捕获冲突。
func (h *InterruptHandler) StartAgentSignalListener() func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		for sig := range sigCh {
			if sig == nil {
				return
			}
			h.mu.Lock()
			if h.cancel != nil {
				h.cancel()
				h.cancel = nil
				h.pressed = true
				h.mu.Unlock()
			} else if h.pressed {
				// agent 运行时第二次 Ctrl+C（cancel 已被调用），强制退出
				h.mu.Unlock()
				os.Exit(130)
			} else {
				h.mu.Unlock()
			}
		}
	}()
	return func() {
		signal.Stop(sigCh)
		close(sigCh)
	}
}
