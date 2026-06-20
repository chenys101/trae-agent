package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RetryableProvider 包装一个 Provider，对可重试错误做指数退避重试。
type RetryableProvider struct {
	inner      Provider
	maxRetries int
	baseDelay  time.Duration
}

// NewRetryable 构造重试包装器。
func NewRetryable(inner Provider, maxRetries int, baseDelay time.Duration) *RetryableProvider {
	if maxRetries < 1 {
		maxRetries = 1
	}
	if baseDelay <= 0 {
		baseDelay = 500 * time.Millisecond
	}
	return &RetryableProvider{inner: inner, maxRetries: maxRetries, baseDelay: baseDelay}
}

func (r *RetryableProvider) Name() string { return r.inner.Name() }

func (r *RetryableProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	var lastErr error
	for attempt := 0; attempt < r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<(attempt-1)) * r.baseDelay
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

// isRetryable 判断错误是否可重试：5xx、429、网络错误。
func isRetryable(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "status 5") || strings.Contains(msg, "503") ||
		strings.Contains(msg, "502") || strings.Contains(msg, "500") ||
		strings.Contains(msg, "429") || strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection") || strings.Contains(msg, "EOF") {
		return true
	}
	return false
}
