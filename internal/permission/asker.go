package permission

import "context"

// Asker 交互式询问接口，当策略返回 ActionAsk 时调用。
type Asker interface {
	Ask(ctx context.Context, tool, args, reason string) Action
}

// AutoAsker 自动决策，用于 headless 模式。
type AutoAsker struct {
	Default Action
}

func (a *AutoAsker) Ask(ctx context.Context, tool, args, reason string) Action {
	return a.Default
}

// CallbackAsker 用回调函数决策，供测试 mock。
type CallbackAsker struct {
	Func func(ctx context.Context, tool, args, reason string) Action
}

func (c *CallbackAsker) Ask(ctx context.Context, tool, args, reason string) Action {
	if c.Func == nil {
		return ActionDeny
	}
	return c.Func(ctx, tool, args, reason)
}
