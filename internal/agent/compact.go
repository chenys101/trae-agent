package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/trae-agent/internal/llm"
)

const compactPrompt = `Summarize the following conversation history. Preserve:
1. The user's main goal/task
2. Key decisions made
3. Files created or modified (with paths)
4. Important errors encountered and how they were resolved
5. Current state and next steps

Be concise but complete. Output a single summary paragraph.`

// Compactor 用 LLM 压缩对话历史。
type Compactor struct {
	provider llm.Provider
	cm       *ContextManager
	model    string // 用于 summarize 请求的 model
}

func NewCompactor(provider llm.Provider, cm *ContextManager, model string) *Compactor {
	return &Compactor{provider: provider, cm: cm, model: model}
}

// Compact 压缩 messages，返回新的 messages 列表。
// 保留最近 compactKeep 条消息，其余用 LLM 生成的摘要替换。
func (c *Compactor) Compact(ctx context.Context, messages []llm.Message) ([]llm.Message, error) {
	if len(messages) <= c.cm.CompactKeep() {
		return messages, nil // 消息太少，不需要压缩
	}

	keep := c.cm.CompactKeep()
	toSummarize := messages[:len(messages)-keep]
	recent := messages[len(messages)-keep:]

	summary, err := c.summarize(ctx, toSummarize)
	if err != nil {
		return nil, fmt.Errorf("compact: %w", err)
	}

	// 构造新历史：摘要消息 + 保留的最近消息
	result := make([]llm.Message, 0, keep+1)
	result = append(result, llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf("[Previous conversation summary]\n%s\n[End of summary. Continue from here.]", summary),
	})
	result = append(result, recent...)
	return result, nil
}

// summarize 调 LLM 生成摘要。
func (c *Compactor) summarize(ctx context.Context, messages []llm.Message) (string, error) {
	// 把历史拼成文本
	var b strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&b, "%s: %s", m.Role, m.Content)
		for _, tc := range m.ToolCalls {
			fmt.Fprintf(&b, " [tool: %s %s]", tc.Name, tc.Args)
		}
		b.WriteString("\n")
	}

	req := llm.Request{
		Model:  c.model,
		System: compactPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: b.String()},
		},
	}

	ch, err := c.provider.Stream(ctx, req)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Content)
		case llm.Error:
			return "", e.Err
		}
	}
	return result.String(), nil
}
