package cli

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// doublePressWindow 双击 Ctrl+C 的有效时间窗口，超过则视为首次按下。
const doublePressWindow = 2 * time.Second

// InterruptHandler 管理 Ctrl+C 信号。
// 第一次 Ctrl+C 中断当前 agent 运行，第二次退出 REPL。
type InterruptHandler struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	pressed   bool
	lastPress time.Time
}

func NewInterruptHandler() *InterruptHandler {
	return &InterruptHandler{}
}

// SetCancel 设置当前 agent 运行的 cancel 函数。
// agent 开始时调 SetCancel(cancel)，结束时调 SetCancel(nil)。
// 仅在设置非 nil cancel 时重置 pressed，避免 SetCancel(nil) 清掉
// 双击状态导致跨 agent 运行的双击失效。
func (h *InterruptHandler) SetCancel(cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancel = cancel
	if cancel != nil {
		h.pressed = false
	}
}

// Handle 处理一次 Ctrl+C 信号。
// 返回 true 表示应退出 REPL，false 表示仅中断当前 agent。
func (h *InterruptHandler) Handle() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	// 超过时间窗口则重置 pressed，视为首次按下
	if h.pressed && now.Sub(h.lastPress) > doublePressWindow {
		h.pressed = false
	}

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
		h.pressed = true
		h.lastPress = now
		return false
	}

	if h.pressed {
		return true
	}
	h.pressed = true
	h.lastPress = now
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
		// range 在 close 后自动退出，无需 nil 检查
		for range sigCh {
			now := time.Now()
			h.mu.Lock()
			// 超过时间窗口则重置 pressed，视为首次按下
			if h.pressed && now.Sub(h.lastPress) > doublePressWindow {
				h.pressed = false
			}
			if h.cancel != nil {
				h.cancel()
				h.cancel = nil
				h.pressed = true
				h.lastPress = now
				h.mu.Unlock()
			} else if h.pressed {
				// agent 运行时第二次 Ctrl+C（cancel 已被调用），强制退出
				h.mu.Unlock()
				os.Exit(130)
			} else {
				h.pressed = true
				h.lastPress = now
				h.mu.Unlock()
			}
		}
	}()
	return func() {
		signal.Stop(sigCh)
		close(sigCh)
	}
}
