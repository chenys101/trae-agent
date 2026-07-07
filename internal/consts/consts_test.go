package consts

import (
	"testing"
	"time"
)

// TestConsts_SanityChecks 对关键常量做合理性断言，
// 防止被误改后失去语义（如缓冲区为 0、超时为 0、上限小于下限等）。
// 这些值本身是契约，变更需显式确认。
func TestConsts_SanityChecks(t *testing.T) {
	// 步数应大于 0，且子 agent 应小于主 agent
	if DefaultMaxSteps <= 0 || DefaultSubagentMaxSteps <= 0 {
		t.Fatalf("max steps must be positive: main=%d sub=%d", DefaultMaxSteps, DefaultSubagentMaxSteps)
	}
	if DefaultSubagentMaxSteps >= DefaultMaxSteps {
		t.Fatalf("subagent steps %d should be < main steps %d", DefaultSubagentMaxSteps, DefaultMaxSteps)
	}

	// 超时必须 > 0
	positiveTimeouts := map[string]time.Duration{
		"DefaultBashTimeout":        DefaultBashTimeout,
		"DefaultRetryBaseDelay":     DefaultRetryBaseDelay,
		"DefaultRetryMaxDelay":      DefaultRetryMaxDelay,
		"HTTPDialTimeout":           HTTPDialTimeout,
		"HTTPTLSHandshakeTimeout":   HTTPTLSHandshakeTimeout,
		"HTTPResponseHeaderTimeout": HTTPResponseHeaderTimeout,
		"MCPRPCTimeout":             MCPRPCTimeout,
		"MCPGracefulCloseTimeout":   MCPGracefulCloseTimeout,
	}
	for name, d := range positiveTimeouts {
		if d <= 0 {
			t.Fatalf("%s must be positive, got %v", name, d)
		}
	}

	// 退避上下限
	if DefaultRetryMaxDelay < DefaultRetryBaseDelay {
		t.Fatalf("max retry delay %v < base %v", DefaultRetryMaxDelay, DefaultRetryBaseDelay)
	}

	// 缓冲 / 上限
	if MaxBashOutput <= 0 || MaxFileSize <= MaxBashOutput {
		t.Fatalf("MaxFileSize %d should be > MaxBashOutput %d", MaxFileSize, MaxBashOutput)
	}
	if ScannerMaxBuf <= ScannerInitialBuf {
		t.Fatalf("ScannerMaxBuf %d should be > ScannerInitialBuf %d", ScannerMaxBuf, ScannerInitialBuf)
	}
	if ToolConcurrency <= 0 || EventBufferSize <= 0 || StreamBufferSize <= 0 {
		t.Fatalf("concurrency/buffer sizes must be positive")
	}

	// 上下文估算
	if TokenEstimateRatio <= 0 || DefaultMaxTokens <= 0 {
		t.Fatalf("token estimate params must be positive")
	}
}
