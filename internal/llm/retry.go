package llm

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// RetryableProvider 包装一个 Provider，对可重试错误做指数退避重试。
type RetryableProvider struct {
	inner Provider
	// maxRetries 实为最大尝试次数（含首次调用），并非"额外重试次数"。
	// 例如 maxRetries=3 表示最多调用 3 次（1 次首发 + 2 次重试）。
	// 字段名保留以维持 API 兼容，语义以本注释为准。
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// NewRetryable 构造重试包装器。maxDelay 默认 30s，对单次退避做上限封顶，
// 避免指数膨胀导致长时间阻塞。
func NewRetryable(inner Provider, maxRetries int, baseDelay time.Duration) *RetryableProvider {
	if maxRetries < 1 {
		maxRetries = 1
	}
	if baseDelay <= 0 {
		baseDelay = 500 * time.Millisecond
	}
	return &RetryableProvider{
		inner:      inner,
		maxRetries: maxRetries,
		baseDelay:  baseDelay,
		maxDelay:   30 * time.Second,
	}
}

func (r *RetryableProvider) Name() string { return r.inner.Name() }

func (r *RetryableProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	var lastErr error
	for attempt := 0; attempt < r.maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避：baseDelay * 2^(attempt-1)，并封顶到 maxDelay。
			delay := time.Duration(1<<(attempt-1)) * r.baseDelay
			if delay > r.maxDelay {
				delay = r.maxDelay
			}
			// 加入 jitter：实际等待区间为 [delay/2, delay)，避免重试风暴。
			half := delay / 2
			if half > 0 {
				delay = half + time.Duration(rand.Int64N(int64(half)))
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ch, err := r.inner.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", r.maxRetries, lastErr)
}

// isRetryable 判断错误是否可重试：5xx、429、网络错误及服务端过载/限流类提示。
// 当前错误多为 fmt.Errorf 包装的字符串，故以字符串匹配为主；
// 同时扩充常见模式以覆盖各供应商的过载/限流文案。
func isRetryable(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "status 5") || strings.Contains(msg, "503") ||
		strings.Contains(msg, "502") || strings.Contains(msg, "500") ||
		strings.Contains(msg, "429") || strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection") || strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "overloaded") || strings.Contains(msg, "rate_limit") ||
		strings.Contains(msg, "service_unavailable") {
		return true
	}
	return false
}
