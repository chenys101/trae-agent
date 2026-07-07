// Package errors 提供统一的业务错误封装，支持错误码和包装链。
// 使用方式：
//
//	err = errors.Wrap(err, errors.CodeSessionLoadFailed, "load session")
//	if errors.Is(err, errors.ErrNotFound) { ... }
package errors

import (
	stderrors "errors"
	"fmt"
)

// Code 业务错误码。
type Code int

const (
	CodeUnknown     Code = 0
	CodeNotFound    Code = 1  // 资源不存在
	CodeInvalidArg  Code = 2  // 参数校验失败
	CodePermission  Code = 3  // 权限拒绝
	CodeConfigLoad  Code = 4  // 配置加载失败
	CodeSessionLoad Code = 5  // 会话加载失败
	CodeSessionSave Code = 6  // 会话保存失败
	CodeLLMRequest  Code = 7  // LLM 请求失败
	CodeLLMStream   Code = 8  // LLM 流式错误
	CodeToolExec    Code = 9  // 工具执行失败
	CodeMCPConnect  Code = 10 // MCP 连接失败
	CodeMCPRPC      Code = 11 // MCP RPC 调用失败
	CodeFileWrite   Code = 12 // 文件写入失败
	CodeFileRead    Code = 13 // 文件读取失败
	CodeTrajectory  Code = 14 // 轨迹记录失败
)

// sentinel errors，供 errors.Is 判断
var (
	ErrNotFound    = newError(CodeNotFound, "not found")
	ErrInvalidArg  = newError(CodeInvalidArg, "invalid argument")
	ErrPermission  = newError(CodePermission, "permission denied")
	ErrConfigLoad  = newError(CodeConfigLoad, "config load failed")
	ErrSessionLoad = newError(CodeSessionLoad, "session load failed")
	ErrSessionSave = newError(CodeSessionSave, "session save failed")
	ErrLLMRequest  = newError(CodeLLMRequest, "llm request failed")
	ErrLLMStream   = newError(CodeLLMStream, "llm stream error")
	ErrToolExec    = newError(CodeToolExec, "tool execution failed")
	ErrMCPConnect  = newError(CodeMCPConnect, "mcp connect failed")
	ErrMCPRPC      = newError(CodeMCPRPC, "mcp rpc failed")
	ErrFileWrite   = newError(CodeFileWrite, "file write failed")
	ErrFileRead    = newError(CodeFileRead, "file read failed")
	ErrTrajectory  = newError(CodeTrajectory, "trajectory record failed")
)

// Error 统一错误类型，包含错误码、消息和包装链。
type Error struct {
	code    Code
	msg     string
	wrapped error
}

func newError(code Code, msg string) *Error {
	return &Error{code: code, msg: msg}
}

// Code 返回错误码。
func (e *Error) Code() Code { return e.code }

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e.wrapped != nil {
		return fmt.Sprintf("%s: %v", e.msg, e.wrapped)
	}
	return e.msg
}

// Unwrap 支持 errors.Is / errors.As 遍历包装链。
func (e *Error) Unwrap() error { return e.wrapped }

// Wrap 包装底层 error，附加错误码和上下文消息。
// 若 err 为 nil 返回 nil。
// 底层 error 通过 Unwrap 链可达，errors.Is 可匹配 sentinel。
func Wrap(err error, code Code, msg string) error {
	if err == nil {
		return nil
	}
	// 已是本包 Error：沿用其业务错误码，并把原 *Error 整体作为 wrapped 保留，
	// 使 errors.Is 可穿透匹配到 sentinel（如 ErrNotFound）。
	if se, ok := err.(*Error); ok {
		return &Error{code: se.code, msg: msg, wrapped: se}
	}
	return &Error{code: code, msg: msg, wrapped: err}
}

// Wrapf 同 Wrap，支持格式化消息。
func Wrapf(err error, code Code, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return Wrap(err, code, fmt.Sprintf(format, args...))
}

// New 创建新的错误（无底层 error）。
func New(code Code, msg string) error {
	return &Error{code: code, msg: msg}
}

// Newf 同 New，支持格式化。
func Newf(code Code, format string, args ...any) error {
	return &Error{code: code, msg: fmt.Sprintf(format, args...)}
}

// Is 代理标准库 errors.Is。
func Is(err, target error) bool { return stderrors.Is(err, target) }

// As 代理标准库 errors.As。
func As(err error, target any) bool { return stderrors.As(err, target) }
