package tool

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/bytedance/trae-agent/internal/consts"
	"github.com/bytedance/trae-agent/internal/permission"
)

// Call 单个工具调用请求。
type Call struct {
	Name string
	Args json.RawMessage
}

// CallResult 工具调用结果，保持与 Call 的顺序。
type CallResult struct {
	Name   string
	Result Result
}

// Dispatcher 调度工具执行，支持并行。
type Dispatcher struct {
	registry *Registry
	policy   permission.Policy
	asker    permission.Asker
}

// NewDispatcher 构造无权限检查的 dispatcher（向后兼容）。
func NewDispatcher(r *Registry) *Dispatcher {
	return &Dispatcher{registry: r}
}

// NewDispatcherWithPolicy 构造带权限策略的 dispatcher，asker 默认为 AutoAsker{Default: Deny}。
func NewDispatcherWithPolicy(r *Registry, p permission.Policy) *Dispatcher {
	return &Dispatcher{
		registry: r,
		policy:   p,
		asker:    &permission.AutoAsker{Default: permission.ActionDeny},
	}
}

// NewDispatcherWithPolicyAndAsker 构造带权限策略和自定义 asker 的 dispatcher。
func NewDispatcherWithPolicyAndAsker(r *Registry, p permission.Policy, a permission.Asker) *Dispatcher {
	return &Dispatcher{
		registry: r,
		policy:   p,
		asker:    a,
	}
}

// SetAsker 运行时注入 asker，供 REPL 在初始化后替换交互式 asker。
func (d *Dispatcher) SetAsker(a permission.Asker) {
	d.asker = a
}

// Dispatch 并行执行所有调用，返回结果（顺序与输入一致）。
func (d *Dispatcher) Dispatch(ctx context.Context, calls []Call) []CallResult {
	results := make([]CallResult, len(calls))
	var wg sync.WaitGroup
	// 用带缓冲 channel 作为 semaphore，限制最多 consts.ToolConcurrency 个并发 goroutine，
	// 避免一次性派生过多 goroutine 导致资源耗尽。
	sem := make(chan struct{}, consts.ToolConcurrency)
	for i, c := range calls {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, call Call) {
			defer wg.Done()
			defer func() { <-sem }()
			// 捕获单个工具 panic，避免崩溃整个进程。
			defer func() {
				if r := recover(); r != nil {
					results[idx] = CallResult{
						Name:   call.Name,
						Result: ErrorResult("tool panic: %v", r),
					}
				}
			}()
			results[idx] = CallResult{
				Name:   call.Name,
				Result: d.execute(ctx, call),
			}
		}(i, c)
	}
	wg.Wait()
	return results
}

func (d *Dispatcher) execute(ctx context.Context, call Call) Result {
	// 权限检查：policy 为 nil 时跳过（向后兼容）
	if d.policy != nil {
		decision := d.policy.Check(call.Name, call.Args)
		switch decision.Action {
		case permission.ActionDeny, permission.ActionDenyAlways:
			return ErrorResult("permission denied: %s", decision.Reason)
		case permission.ActionAsk:
			// asker 为 nil 时安全起见拒绝
			if d.asker == nil {
				return ErrorResult("permission denied: no asker configured for %s", decision.Reason)
			}
			askDecision := d.asker.Ask(ctx, call.Name, string(call.Args), decision.Reason)
			switch askDecision {
			case permission.ActionAllow, permission.ActionAllowAlways:
				// 继续执行
			case permission.ActionDeny, permission.ActionDenyAlways, permission.ActionAsk:
				// asker 不应返回 Ask，但安全起见也拒绝
				return ErrorResult("permission denied by user: %s", decision.Reason)
			}
		case permission.ActionAllow, permission.ActionAllowAlways:
			// 继续执行
		}
	}

	t, ok := d.registry.Get(call.Name)
	if !ok {
		return ErrorResult("unknown tool: %s", call.Name)
	}
	return t.Run(ctx, call.Args)
}
