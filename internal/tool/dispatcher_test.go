package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatcher_sequential(t *testing.T) {
	r := NewRegistry(echoTool{})
	d := NewDispatcher(r)

	calls := d.Dispatch(context.Background(), []Call{
		{Name: "echo", Args: json.RawMessage(`{"msg":"a"}`)},
		{Name: "echo", Args: json.RawMessage(`{"msg":"b"}`)},
	})
	if len(calls) != 2 {
		t.Fatalf("got %d results, want 2", len(calls))
	}
	if calls[0].Result.Content != "a" {
		t.Errorf("result[0] = %q, want a", calls[0].Result.Content)
	}
	if calls[1].Result.Content != "b" {
		t.Errorf("result[1] = %q, want b", calls[1].Result.Content)
	}
}

func TestDispatcher_parallel(t *testing.T) {
	// 用慢工具验证并行
	slow := &slowTool{delay: 100 * time.Millisecond}
	r := NewRegistry(slow)
	d := NewDispatcher(r)

	start := time.Now()
	calls := d.Dispatch(context.Background(), []Call{
		{Name: "slow", Args: json.RawMessage(`{}`)},
		{Name: "slow", Args: json.RawMessage(`{}`)},
		{Name: "slow", Args: json.RawMessage(`{}`)},
	})
	elapsed := time.Since(start)
	if len(calls) != 3 {
		t.Fatalf("got %d results, want 3", len(calls))
	}
	// 并行 3 个 100ms 应 < 300ms（串行会 > 300ms）
	if elapsed > 250*time.Millisecond {
		t.Errorf("not parallel: elapsed %v", elapsed)
	}
}

func TestDispatcher_unknownTool(t *testing.T) {
	r := NewRegistry(echoTool{})
	d := NewDispatcher(r)
	calls := d.Dispatch(context.Background(), []Call{
		{Name: "nope", Args: json.RawMessage(`{}`)},
	})
	if len(calls) != 1 {
		t.Fatal("expected 1 result")
	}
	if !calls[0].Result.IsError {
		t.Error("expected error for unknown tool")
	}
}

type slowTool struct {
	delay time.Duration
	calls int32
}

func (s *slowTool) Name() string            { return "slow" }
func (s *slowTool) Description() string     { return "slow tool" }
func (s *slowTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (s *slowTool) Run(ctx context.Context, args json.RawMessage) Result {
	atomic.AddInt32(&s.calls, 1)
	time.Sleep(s.delay)
	return Result{Content: "done"}
}

// panicTool 模拟运行时 panic 的工具，用于验证 dispatcher 的 recover 机制。
type panicTool struct{}

func (panicTool) Name() string            { return "panic" }
func (panicTool) Description() string     { return "a tool that panics" }
func (panicTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (panicTool) Run(ctx context.Context, args json.RawMessage) Result {
	panic("boom")
}

// blockingTool 阻塞直到 ctx 取消，用于验证 dispatcher 在 ctx 取消时不会永久阻塞。
type blockingTool struct{}

func (blockingTool) Name() string            { return "block" }
func (blockingTool) Description() string     { return "a tool that blocks until ctx cancel" }
func (blockingTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (blockingTool) Run(ctx context.Context, args json.RawMessage) Result {
	<-ctx.Done()
	return ErrorResult("cancelled")
}

// concurrencyTool 跟踪当前并发数与峰值并发数，用于验证 semaphore 上限。
type concurrencyTool struct {
	current int32
	peak    int32
	delay   time.Duration
}

func (c *concurrencyTool) Name() string            { return "concurrency" }
func (c *concurrencyTool) Description() string     { return "a tool that tracks concurrency" }
func (c *concurrencyTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (c *concurrencyTool) Run(ctx context.Context, args json.RawMessage) Result {
	cur := atomic.AddInt32(&c.current, 1)
	// 原子更新峰值
	for {
		p := atomic.LoadInt32(&c.peak)
		if cur <= p {
			break
		}
		if atomic.CompareAndSwapInt32(&c.peak, p, cur) {
			break
		}
	}
	time.Sleep(c.delay)
	atomic.AddInt32(&c.current, -1)
	return Result{Content: "done"}
}

func TestDispatcher_panicRecovery(t *testing.T) {
	r := NewRegistry(panicTool{})
	d := NewDispatcher(r)
	calls := d.Dispatch(context.Background(), []Call{
		{Name: "panic", Args: json.RawMessage(`{}`)},
	})
	if len(calls) != 1 {
		t.Fatalf("got %d results, want 1", len(calls))
	}
	if !calls[0].Result.IsError {
		t.Error("expected error result from panic")
	}
	if !strings.Contains(calls[0].Result.Content, "panic") {
		t.Errorf("expected 'panic' in content, got: %s", calls[0].Result.Content)
	}
}

func TestDispatcher_contextCancel(t *testing.T) {
	r := NewRegistry(blockingTool{})
	d := NewDispatcher(r)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	calls := d.Dispatch(ctx, []Call{
		{Name: "block", Args: json.RawMessage(`{}`)},
	})
	elapsed := time.Since(start)
	if len(calls) != 1 {
		t.Fatalf("got %d results, want 1", len(calls))
	}
	// 验证 Dispatch 在 ctx 取消后及时返回，不永久阻塞
	if elapsed > 5*time.Second {
		t.Errorf("dispatch blocked too long: %v", elapsed)
	}
	if !calls[0].Result.IsError {
		t.Error("expected error from cancelled tool")
	}
}

func TestDispatcher_emptyCalls(t *testing.T) {
	r := NewRegistry(echoTool{})
	d := NewDispatcher(r)
	calls := d.Dispatch(context.Background(), []Call{})
	if len(calls) != 0 {
		t.Errorf("got %d results, want 0", len(calls))
	}
}

func TestDispatcher_concurrencyLimit(t *testing.T) {
	tool := &concurrencyTool{delay: 100 * time.Millisecond}
	r := NewRegistry(tool)
	d := NewDispatcher(r)
	calls := make([]Call, 20)
	for i := range calls {
		calls[i] = Call{Name: "concurrency", Args: json.RawMessage(`{}`)}
	}
	results := d.Dispatch(context.Background(), calls)
	if len(results) != 20 {
		t.Fatalf("got %d results, want 20", len(results))
	}
	peak := atomic.LoadInt32(&tool.peak)
	// semaphore=8，峰值并发不应超过 8
	if peak > 8 {
		t.Errorf("peak concurrency = %d, want <= 8", peak)
	}
	// 验证确实发生了并行（peak > 1）
	if peak < 2 {
		t.Errorf("peak concurrency = %d, expected some parallelism", peak)
	}
}
