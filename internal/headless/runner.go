package headless

import (
	"context"
	"fmt"
	"io"

	"github.com/bytedance/trae-agent/internal/llm"
)

// Run 执行单轮 LLM 调用，流式写入 out，返回 usage 统计。
func Run(ctx context.Context, provider llm.Provider, req llm.Request, out io.Writer) (llm.Usage, error) {
	ch, err := provider.Stream(ctx, req)
	if err != nil {
		return llm.Usage{}, fmt.Errorf("provider stream: %w", err)
	}

	var usage llm.Usage
	for event := range ch {
		switch e := event.(type) {
		case llm.TextDelta:
			if _, err := io.WriteString(out, e.Content); err != nil {
				return usage, fmt.Errorf("write output: %w", err)
			}
		case llm.Done:
			usage = e.Usage
		case llm.Error:
			return usage, e.Err
		}
	}
	return usage, nil
}
