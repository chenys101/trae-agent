package agent

import (
	"unicode/utf8"

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
// 用 utf8.RuneCountInString 计字符数：中文约 1 token/字、英文约 0.25 token/字，
// 混合估算取平均每字符 ~0.5 token（rune 数 / 2）。
// 注意：这是包级函数而非 ContextManager 方法，repl.go 直接调用以展示压缩前后 token 数。
// 该估算偏保守，未来可替换为真实 tokenizer（如 tiktoken）以提升精度。
func EstimateTokens(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		runes := utf8.RuneCountInString(m.Content)
		// 混合估算：假设中英混合，平均每字符 ~0.5 token
		total += runes / 2
		for _, tc := range m.ToolCalls {
			total += utf8.RuneCountInString(tc.Name) / 2
			total += utf8.RuneCountInString(tc.Args) / 2
		}
	}
	// 每条消息固定开销（role 标记等）
	total += len(messages) * 4
	return total
}

// ShouldCompact 检查是否需要压缩。
// 用 >= 而非 >：达到阈值即触发，边界行为更直观。
func (c *ContextManager) ShouldCompact(messages []llm.Message) bool {
	return EstimateTokens(messages) >= c.maxTokens
}

// MaxTokens 返回阈值。
func (c *ContextManager) MaxTokens() int { return c.maxTokens }

// CompactKeep 返回保留消息数。
func (c *ContextManager) CompactKeep() int { return c.compactKeep }
