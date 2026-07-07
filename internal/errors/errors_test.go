package errors

import (
	stderrors "errors"
	"fmt"
	"testing"
)

// TestWrap_NilInput 验证 Wrap 对 nil 输入直接返回 nil，避免产生空错误。
func TestWrap_NilInput(t *testing.T) {
	if got := Wrap(nil, CodeUnknown, "should be nil"); got != nil {
		t.Fatalf("Wrap(nil, ...) = %v, want nil", got)
	}
}

// TestWrap_WithStdError 验证包装标准库 error 后：错误码、消息、Unwrap 链均正确。
func TestWrap_WithStdError(t *testing.T) {
	base := stderrors.New("disk full")
	wrapped := Wrap(base, CodeConfigLoad, "load config")

	// 错误码
	var ce *Error
	if !stderrors.As(wrapped, &ce) {
		t.Fatalf("wrapped should be *errors.Error")
	}
	if ce.Code() != CodeConfigLoad {
		t.Fatalf("code = %d, want %d", ce.Code(), CodeConfigLoad)
	}

	// 消息格式 "<msg>: <wrapped>"
	if ce.Error() != "load config: disk full" {
		t.Fatalf("Error() = %q, want %q", ce.Error(), "load config: disk full")
	}

	// Unwrap 链可达原始 error
	if !stderrors.Is(wrapped, base) {
		t.Fatalf("errors.Is(wrapped, base) = false, want true")
	}
}

// TestWrap_AlreadyWrapped 验证对已封装过的 error 再次 Wrap 时，
// 沿用原错误码，仅追加上下文消息，避免覆盖业务码。
func TestWrap_AlreadyWrapped(t *testing.T) {
	first := Wrap(stderrors.New("io err"), CodeFileRead, "read file")
	second := Wrap(first, CodeUnknown, "outer") // CodeUnknown 应被忽略

	var ce *Error
	if !stderrors.As(second, &ce) {
		t.Fatalf("second should be *errors.Error")
	}
	if ce.Code() != CodeFileRead {
		t.Fatalf("code = %d, want CodeFileRead=%d", ce.Code(), CodeFileRead)
	}
	if ce.Error() != "outer: read file: io err" {
		t.Fatalf("Error() = %q, want %q", ce.Error(), "outer: read file: io err")
	}
}

// TestIs_MatchesSentinel 验证 errors.Is 可穿透 Wrap 链匹配 sentinel。
func TestIs_MatchesSentinel(t *testing.T) {
	err := Wrap(ErrNotFound, CodeUnknown, "load session")
	if !Is(err, ErrNotFound) {
		t.Fatalf("Is(err, ErrNotFound) = false, want true")
	}
}

// TestNewf 验证 Newf 格式化消息。
func TestNewf(t *testing.T) {
	err := Newf(CodeInvalidArg, "bad value %d", 42)
	var ce *Error
	if !stderrors.As(err, &ce) || ce.Code() != CodeInvalidArg {
		t.Fatalf("Newf code mismatch: %v", err)
	}
	if err.Error() != "bad value 42" {
		t.Fatalf("Error() = %q, want %q", err.Error(), "bad value 42")
	}
}

// TestWrapf 验证 Wrapf 格式化消息并保留底层 error。
func TestWrapf(t *testing.T) {
	base := fmt.Errorf("network unreachable")
	err := Wrapf(base, CodeLLMRequest, "call %s after %d retries", "anthropic", 3)

	var ce *Error
	if !stderrors.As(err, &ce) || ce.Code() != CodeLLMRequest {
		t.Fatalf("Wrapf code mismatch: %v", err)
	}
	if !Is(err, base) {
		t.Fatalf("Wrapf should preserve underlying error via Unwrap")
	}
	if err.Error() != "call anthropic after 3 retries: network unreachable" {
		t.Fatalf("Error() = %q", err.Error())
	}
}
