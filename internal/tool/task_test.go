package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTask_singleSubagent(t *testing.T) {
	runner := &mockTaskRunner{
		results: map[string]string{
			"search": "found 3 TODOs in the codebase",
		},
	}
	task := NewTask(runner)

	args, _ := json.Marshal(map[string]string{
		"subagent_type": "search",
		"description":   "find TODOs",
		"prompt":        "find all TODO comments",
	})

	res := task.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "found 3 TODOs") {
		t.Errorf("result should contain subagent output: %s", res.Content)
	}
}

func TestTask_parallelSubagents(t *testing.T) {
	var peakConcurrent int32
	var current int32

	runner := &mockTaskRunner{
		results: map[string]string{
			"search":          "search done",
			"general-purpose": "general done",
		},
		onRun: func() {
			cur := atomic.AddInt32(&current, 1)
			for {
				peak := atomic.LoadInt32(&peakConcurrent)
				if cur <= peak || atomic.CompareAndSwapInt32(&peakConcurrent, peak, cur) {
					break
				}
			}
		},
	}
	task := NewTask(runner)

	d := NewDispatcher(NewRegistry(task))
	calls := []Call{
		{Name: "task", Args: mustMarshal(t, map[string]string{
			"subagent_type": "search",
			"description":   "search task",
			"prompt":        "search something",
		})},
		{Name: "task", Args: mustMarshal(t, map[string]string{
			"subagent_type": "general-purpose",
			"description":   "general task",
			"prompt":        "do something",
		})},
	}

	results := d.Dispatch(context.Background(), calls)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Result.IsError {
			t.Errorf("unexpected error: %s", r.Result.Content)
		}
	}
}

func TestTask_missingSubagentType(t *testing.T) {
	runner := &mockTaskRunner{}
	task := NewTask(runner)

	args, _ := json.Marshal(map[string]string{
		"description": "no type",
		"prompt":      "test",
	})

	res := task.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for missing subagent_type")
	}
}

func TestTask_unknownSubagentType(t *testing.T) {
	runner := &mockTaskRunner{
		errs: map[string]error{
			"unknown": errUnknownType,
		},
	}
	task := NewTask(runner)

	args, _ := json.Marshal(map[string]string{
		"subagent_type": "unknown",
		"description":   "bad type",
		"prompt":        "test",
	})

	res := task.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for unknown subagent_type")
	}
}

func TestTask_contextCancel(t *testing.T) {
	runner := &mockTaskRunner{
		blockCtx: true,
	}
	task := NewTask(runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	args, _ := json.Marshal(map[string]string{
		"subagent_type": "search",
		"description":   "cancelled",
		"prompt":        "test",
	})

	res := task.Run(ctx, args)
	if !res.IsError {
		t.Error("expected error for cancelled context")
	}
}

// mockTaskRunner mock TaskRunner 接口
type mockTaskRunner struct {
	results  map[string]string
	errs     map[string]error
	onRun    func()
	blockCtx bool
}

func (m *mockTaskRunner) RunSubagent(ctx context.Context, subagentType, description, prompt string) (string, error) {
	if m.blockCtx {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if m.onRun != nil {
		m.onRun()
	}
	if err, ok := m.errs[subagentType]; ok {
		return "", err
	}
	if result, ok := m.results[subagentType]; ok {
		return result, nil
	}
	return "default mock result", nil
}

var errUnknownType = &mockErr{"unknown subagent type"}

type mockErr struct{ msg string }

func (e *mockErr) Error() string { return e.msg }

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
