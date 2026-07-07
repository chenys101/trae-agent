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
	recent, toSummarize := splitForCompact(messages, keep)

	summary, err := c.summarize(ctx, toSummarize)
	if err != nil {
		// 降级：直接截断，保留 recent，丢弃 toSummarize，避免失败风暴
		result := make([]llm.Message, 0, len(recent)+2)
		result = append(result, llm.Message{
			Role:    llm.RoleUser,
			Content: "[Previous conversation truncated due to compaction failure]",
		})
		result = append(result, llm.Message{Role: llm.RoleAssistant, Content: "Understood."})
		result = append(result, recent...)
		return result, nil
	}

	// 构造新历史：摘要消息 + 保留的最近消息
	// 摘要用 user 角色，紧跟一个空 assistant 占位，避免 recent[0] 也是 user 时
	// 出现连续 user，或 recent[0] 是 tool 时出现 user→tool 非法序列
	result := make([]llm.Message, 0, len(recent)+2)
	result = append(result, llm.Message{
		Role:    llm.RoleUser,
		Content: fmt.Sprintf("[Previous conversation summary]\n%s\n[End of summary. Continue from here.]", summary),
	})
	result = append(result, llm.Message{Role: llm.RoleAssistant, Content: "Understood. I'll continue from the summarized context."})
	result = append(result, recent...)
	return result, nil
}

// splitForCompact 把 messages 切成 (recent, toSummarize)。
// 切分点会对齐 tool_call/tool_result 配对边界：
//   - 若 recent[0] 是 RoleTool（其 tool_call 落在 toSummarize 侧），把该 tool 消息
//     及其之前所有连续 tool 消息一起移到 toSummarize 侧
//   - 若 toSummarize 末尾是带 ToolCalls 的 assistant，但其 tool result 落在 recent 侧，
//     把该 assistant 消息移到 recent 侧
func splitForCompact(messages []llm.Message, keep int) (recent, toSummarize []llm.Message) {
	split := len(messages) - keep
	if split <= 0 {
		return messages, nil
	}
	// 向前调整：若 recent 第一条是 RoleTool，说明其对应的 assistant(tool_calls) 在 toSummarize 末尾
	// 把 recent 开头连续的 RoleTool 消息移到 toSummarize
	for split < len(messages) && messages[split].Role == llm.RoleTool {
		split++
	}
	// 向后调整：若 toSummarize 末尾是带 ToolCalls 的 assistant，但其 tool result 在 recent 侧
	// 把该 assistant 移到 recent
	for split > 0 && len(messages[split-1].ToolCalls) > 0 {
		split--
	}
	return messages[split:], messages[:split]
}

// summarize 调 LLM 生成摘要。
func (c *Compactor) summarize(ctx context.Context, messages []llm.Message) (string, error) {
	// 把历史拼成文本
	var b strings.Builder
	for _, m := range messages {
		switch m.Role {
		case llm.RoleTool:
			// 标注 ToolCallID，让摘要能关联 tool result 与其 tool_call
			fmt.Fprintf(&b, "tool(result for %s): %s", m.ToolCallID, m.Content)
		default:
			fmt.Fprintf(&b, "%s: %s", m.Role, m.Content)
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, " [tool: %s %s]", tc.Name, tc.Args)
			}
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
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Content)
		case llm.Done:
			// 摘要完成，继续等 channel 关闭
		case llm.Error:
			// 排空 channel，避免发送 goroutine 阻塞泄漏
			go func() {
				for range ch {
				}
			}()
			return "", e.Err
		}
	}
	return result.String(), nil
}
