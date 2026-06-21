package agent

import (
	"github.com/bytedance/trae-agent/internal/llm"
)

// ContextManager 管理对话上下文的 token 计数与压缩阈值。
type ContextManager struct {
	maxTokens   int // 触发 compact 的阈值
	compactKeep int // compact 后保留的最近消息数
}

type ContextOption func(*ContextManager)

func WithMaxTokens(n int) ContextOption {
	return func(c *ContextManager) { c.maxTokens = n }
}

func WithCompactKeep(n int) ContextOption {
	return func(c *ContextManager) { c.compactKeep = n }
}

func NewContextManager(opts ...ContextOption) *ContextManager {
	c := &ContextManager{
		maxTokens:   100000, // 默认 10 万 token 触发
		compactKeep: 6,      // 默认保留最近 6 条消息（3 轮）
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// EstimateTokens 粗略估算 messages 的 token 数。
// 经验值：1 token ≈ 4 字符（英文），中文约 1 token/字。
// 这里用字符数 * 10 / 32 作为近似（偏保守，倾向早 compact）。
// 用 *10/32 而非 /3 避免短消息整数除法归零。
func EstimateTokens(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content) * 10 / 32
		for _, tc := range m.ToolCalls {
			total += len(tc.Name) * 10 / 32
			total += len(tc.Args) * 10 / 32
		}
	}
	// 每条消息固定开销（role 标记等）
	total += len(messages) * 4
	return total
}

// ShouldCompact 检查是否需要压缩。
func (c *ContextManager) ShouldCompact(messages []llm.Message) bool {
	return EstimateTokens(messages) > c.maxTokens
}

// MaxTokens 返回阈值。
func (c *ContextManager) MaxTokens() int { return c.maxTokens }

// CompactKeep 返回保留消息数。
func (c *ContextManager) CompactKeep() int { return c.compactKeep }
