package tool

import (
	"context"
	"sync"
)

// Call 单个工具调用请求。
type Call struct {
	Name string
	Args []byte // json.RawMessage
}

// CallResult 工具调用结果，保持与 Call 的顺序。
type CallResult struct {
	Name   string
	Result Result
}

// Dispatcher 调度工具执行，支持并行。
type Dispatcher struct {
	registry *Registry
}

func NewDispatcher(r *Registry) *Dispatcher {
	return &Dispatcher{registry: r}
}

// Dispatch 并行执行所有调用，返回结果（顺序与输入一致）。
func (d *Dispatcher) Dispatch(ctx context.Context, calls []Call) []CallResult {
	results := make([]CallResult, len(calls))
	var wg sync.WaitGroup
	// 用带缓冲 channel 作为 semaphore，限制最多 8 个并发 goroutine，
	// 避免一次性派生过多 goroutine 导致资源耗尽。
	sem := make(chan struct{}, 8)
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
	t, ok := d.registry.Get(call.Name)
	if !ok {
		return ErrorResult("unknown tool: %s", call.Name)
	}
	return t.Run(ctx, call.Args)
}
