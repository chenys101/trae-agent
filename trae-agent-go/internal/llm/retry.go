// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package llm

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// RetryConfig 定义重试机制的配置。
type RetryConfig struct {
	MaxRetries     int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	RetryableCheck func(statusCode int) bool
}

// DefaultRetryConfig 返回默认的重试配置。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   10,
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		RetryableCheck: func(statusCode int) bool {
			return statusCode == 429 || (statusCode >= 500 && statusCode < 600)
		},
	}
}

// WithRetry 实现指数退避重试机制。
// 每次重试延迟 = min(InitialDelay * 2^attempt, MaxDelay) + random jitter
func WithRetry(ctx context.Context, cfg RetryConfig, fn func() (LLMResponse, error)) (LLMResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// 检查 context 是否已取消
		if ctx.Err() != nil {
			return LLMResponse{}, ctx.Err()
		}

		resp, err := fn()
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// 如果是 HTTP 错误，检查是否可重试
		httpErr, ok := err.(*HTTPError)
		if !ok || !cfg.RetryableCheck(httpErr.StatusCode) {
			return LLMResponse{}, err
		}

		// 已经达到最大重试次数，不再等待
		if attempt == cfg.MaxRetries {
			break
		}

		// 计算指数退避延迟
		delay := cfg.InitialDelay * time.Duration(1<<uint(attempt))
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
		// 添加随机 jitter (0 ~ 1秒)
		jitter := time.Duration(rand.Int63n(int64(time.Second)))
		delay += jitter

		// 等待延迟或 context 取消
		select {
		case <-ctx.Done():
			return LLMResponse{}, ctx.Err()
		case <-time.After(delay):
		}
	}

	return LLMResponse{}, lastErr
}

// HTTPError 表示 HTTP 请求错误，携带状态码信息用于重试判断。
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP error %d: %s", e.StatusCode, e.Message)
}
