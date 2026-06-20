package tool

import (
	"context"
	"encoding/json"
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

func (s *slowTool) Name() string             { return "slow" }
func (s *slowTool) Description() string      { return "slow tool" }
func (s *slowTool) Schema() json.RawMessage  { return json.RawMessage(`{}`) }
func (s *slowTool) Run(ctx context.Context, args json.RawMessage) Result {
	atomic.AddInt32(&s.calls, 1)
	time.Sleep(s.delay)
	return Result{Content: "done"}
}
