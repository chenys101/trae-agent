// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 10 {
		t.Errorf("expected MaxRetries 10, got %d", cfg.MaxRetries)
	}
	if cfg.InitialDelay != 1*time.Second {
		t.Errorf("expected InitialDelay 1s, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 60*time.Second {
		t.Errorf("expected MaxDelay 60s, got %v", cfg.MaxDelay)
	}
	if cfg.RetryableCheck == nil {
		t.Error("expected RetryableCheck to be set")
	}
	if !cfg.RetryableCheck(429) {
		t.Error("expected 429 to be retryable")
	}
	if !cfg.RetryableCheck(500) {
		t.Error("expected 500 to be retryable")
	}
	if !cfg.RetryableCheck(503) {
		t.Error("expected 503 to be retryable")
	}
	if cfg.RetryableCheck(400) {
		t.Error("expected 400 to not be retryable")
	}
	if cfg.RetryableCheck(200) {
		t.Error("expected 200 to not be retryable")
	}
}

func TestWithRetrySuccess(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		RetryableCheck: func(statusCode int) bool {
			return statusCode == 429 || (statusCode >= 500 && statusCode < 600)
		},
	}

	callCount := 0
	resp, err := WithRetry(context.Background(), cfg, func() (LLMResponse, error) {
		callCount++
		return LLMResponse{Content: "ok", StopReason: "end_turn"}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content 'ok', got '%s'", resp.Content)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestWithRetryFailThenSucceed(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		RetryableCheck: func(statusCode int) bool {
			return statusCode == 429
		},
	}

	callCount := 0
	resp, err := WithRetry(context.Background(), cfg, func() (LLMResponse, error) {
		callCount++
		if callCount < 3 {
			return LLMResponse{}, &HTTPError{StatusCode: 429, Message: "rate limited"}
		}
		return LLMResponse{Content: "ok", StopReason: "end_turn"}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected content 'ok', got '%s'", resp.Content)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestWithRetryMaxRetriesExceeded(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		RetryableCheck: func(statusCode int) bool {
			return statusCode == 500
		},
	}

	callCount := 0
	_, err := WithRetry(context.Background(), cfg, func() (LLMResponse, error) {
		callCount++
		return LLMResponse{}, &HTTPError{StatusCode: 500, Message: "internal error"}
	})

	if err == nil {
		t.Fatal("expected error")
	}
	// attempt 0, 1, 2 = 3 calls total (MaxRetries=2 means up to 2 retries after first attempt)
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestWithRetryContextCancelled(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		RetryableCheck: func(statusCode int) bool {
			return statusCode == 429
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := WithRetry(ctx, cfg, func() (LLMResponse, error) {
		return LLMResponse{}, &HTTPError{StatusCode: 429, Message: "rate limited"}
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestWithRetryNonRetryableError(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		RetryableCheck: func(statusCode int) bool {
			return statusCode == 429
		},
	}

	callCount := 0
	_, err := WithRetry(context.Background(), cfg, func() (LLMResponse, error) {
		callCount++
		return LLMResponse{}, &HTTPError{StatusCode: 400, Message: "bad request"}
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (non-retryable), got %d", callCount)
	}
}

func TestWithRetryGenericError(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		RetryableCheck: func(statusCode int) bool {
			return true
		},
	}

	callCount := 0
	_, err := WithRetry(context.Background(), cfg, func() (LLMResponse, error) {
		callCount++
		return LLMResponse{}, errors.New("generic error")
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (non-HTTPError), got %d", callCount)
	}
}

func TestHTTPError(t *testing.T) {
	err := &HTTPError{StatusCode: 429, Message: "rate limited"}
	expected := "HTTP error 429: rate limited"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}
}
