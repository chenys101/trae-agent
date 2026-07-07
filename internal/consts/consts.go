// Package consts 集中管理跨包共享的魔法数字，避免散落在各文件中。
package consts

import "time"

// === 默认值 ===

const (
	// DefaultMaxSteps agent 最大步数。
	DefaultMaxSteps = 20

	// DefaultSubagentMaxSteps 子 agent 最大步数。
	DefaultSubagentMaxSteps = 10

	// DefaultBashTimeout 默认 bash/shell 命令超时。
	DefaultBashTimeout = 120 * time.Second

	// DefaultRetryBaseDelay 重试初始退避。
	DefaultRetryBaseDelay = 500 * time.Millisecond

	// DefaultRetryMaxDelay 重试最大退避。
	DefaultRetryMaxDelay = 30 * time.Second

	// DefaultMaxRetries 默认重试次数。
	DefaultMaxRetries = 3
)

// === 限制 ===

const (
	// MaxBashOutput bash 输出上限（1MB）。
	MaxBashOutput = 1 << 20

	// MaxBashTimeoutMs bash 超时上限（600s）。
	MaxBashTimeoutMs = 600000

	// MaxFileSize 文件大小上限（10MB），read/write/edit 工具用。
	MaxFileSize = 10 << 20

	// EventBufferSize 事件 channel 缓冲大小。
	EventBufferSize = 64

	// StreamBufferSize SSE 流 channel 缓冲大小。
	StreamBufferSize = 16

	// ScannerInitialBuf scanner 初始缓冲区。
	ScannerInitialBuf = 64 * 1024

	// ScannerMaxBuf scanner 最大缓冲区（10MB，支持大 tool_call 参数）。
	ScannerMaxBuf = 10 * 1024 * 1024

	// ToolConcurrency 工具并行执行上限。
	ToolConcurrency = 8

	// APIErrorLimit 读取 API 错误响应体上限（4KB）。
	APIErrorLimit = 4 * 1024
)

// === 上下文管理 ===

const (
	// DefaultMaxTokens 默认上下文 token 阈值。
	DefaultMaxTokens = 100000

	// DefaultCompactKeep compact 默认保留消息数。
	DefaultCompactKeep = 6

	// TokenEstimateRatio token 估算系数（rune / 2）。
	TokenEstimateRatio = 2

	// MsgOverheadTokens 每条消息固定开销 token 数。
	MsgOverheadTokens = 4
)

// === HTTP 超时 ===

const (
	HTTPDialTimeout           = 30 * time.Second
	HTTPTLSHandshakeTimeout   = 10 * time.Second
	HTTPResponseHeaderTimeout = 30 * time.Second

	// MCPRPCTimeout MCP RPC 请求超时。
	MCPRPCTimeout = 30 * time.Second

	// MCPGracefulCloseTimeout MCP 关闭优雅等待。
	MCPGracefulCloseTimeout = 2 * time.Second
)

// === 显示 ===

const (
	// ArgsDisplayLimit 权限询问时 args 显示截断长度。
	ArgsDisplayLimit = 200

	// DescDisplayWidth /help 中描述显示宽度。
	DescDisplayWidth = 60

	// CostDisplayThreshold cost 显示阈值。
	CostDisplayThreshold = 0.01
)
