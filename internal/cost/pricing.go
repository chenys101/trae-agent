package cost

import (
	"fmt"
	"strings"

	"github.com/bytedance/trae-agent/internal/llm"
)

// Price 模型定价（每百万 token，USD）。
type Price struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

var priceTable = map[string]Price{
	"claude-3-5-sonnet": {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	"claude-3-5-haiku":  {InputPerMillion: 0.8, OutputPerMillion: 4.0},
	"claude-3-opus":     {InputPerMillion: 15.0, OutputPerMillion: 75.0},
	"claude-3-sonnet":   {InputPerMillion: 3.0, OutputPerMillion: 15.0},
	"claude-3-haiku":    {InputPerMillion: 0.25, OutputPerMillion: 1.25},
	"gpt-4o":            {InputPerMillion: 2.5, OutputPerMillion: 10.0},
	"gpt-4o-mini":       {InputPerMillion: 0.15, OutputPerMillion: 0.6},
	"gpt-4-turbo":       {InputPerMillion: 10.0, OutputPerMillion: 30.0},
	"gpt-4":             {InputPerMillion: 30.0, OutputPerMillion: 60.0},
	"gpt-3.5":           {InputPerMillion: 0.5, OutputPerMillion: 1.5},
}

// Estimate 根据模型名和 usage 估算费用（USD）。
func Estimate(model string, usage llm.Usage) (float64, bool) {
	if model == "" {
		return 0, false
	}
	lower := strings.ToLower(model)
	for i := len(lower); i > 0; i-- {
		if price, ok := priceTable[lower[:i]]; ok {
			return float64(usage.InputTokens)*price.InputPerMillion/1e6 +
				float64(usage.OutputTokens)*price.OutputPerMillion/1e6, true
		}
	}
	return 0, false
}

// FormatCost 格式化费用为可读字符串。
func FormatCost(cost float64) string {
	if cost < 0.01 {
		return fmt.Sprintf("$%.4f", cost)
	}
	return fmt.Sprintf("$%.2f", cost)
}
