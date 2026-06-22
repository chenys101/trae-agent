package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestTodo_writeAndReadBack(t *testing.T) {
	todo := NewTodo()
	args := mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"content": "写测试", "status": "completed", "priority": "high"},
			{"content": "写实现", "status": "in_progress", "priority": "high"},
			{"content": "注册工具", "status": "pending", "priority": "medium"},
		},
	})
	res := todo.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "写测试") {
		t.Errorf("missing 写测试: %s", res.Content)
	}
	if !strings.Contains(res.Content, "写实现") {
		t.Errorf("missing 写实现: %s", res.Content)
	}
	if !strings.Contains(res.Content, "注册工具") {
		t.Errorf("missing 注册工具: %s", res.Content)
	}
}

func TestTodo_emptyList(t *testing.T) {
	todo := NewTodo()
	// 先写入一些
	args := mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"content": "任务A", "status": "pending", "priority": "low"},
		},
	})
	res := todo.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// 清空
	args = mustJSON(t, map[string]any{"todos": []map[string]string{}})
	res = todo.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error clearing: %s", res.Content)
	}
	if strings.TrimSpace(res.Content) == "" {
		t.Errorf("empty list should still produce a readable message, got: %q", res.Content)
	}
}

func TestTodo_invalidStatus(t *testing.T) {
	todo := NewTodo()
	args := mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"content": "任务", "status": "done", "priority": "high"},
		},
	})
	res := todo.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for invalid status")
	}
	if !strings.Contains(res.Content, "status") {
		t.Errorf("error should mention status: %s", res.Content)
	}
}

func TestTodo_invalidPriority(t *testing.T) {
	todo := NewTodo()
	args := mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"content": "任务", "status": "pending", "priority": "urgent"},
		},
	})
	res := todo.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for invalid priority")
	}
	if !strings.Contains(res.Content, "priority") {
		t.Errorf("error should mention priority: %s", res.Content)
	}
}

func TestTodo_missingContent(t *testing.T) {
	todo := NewTodo()
	args := mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"status": "pending", "priority": "high"},
		},
	})
	res := todo.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for missing content")
	}
}

func TestTodo_multipleInProgress(t *testing.T) {
	todo := NewTodo()
	args := mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"content": "任务A", "status": "in_progress", "priority": "high"},
			{"content": "任务B", "status": "in_progress", "priority": "high"},
		},
	})
	res := todo.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for multiple in_progress items")
	}
}

func TestTodo_replaceEntireList(t *testing.T) {
	todo := NewTodo()
	// 第一次写入 3 项
	args := mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"content": "旧任务1", "status": "pending", "priority": "low"},
			{"content": "旧任务2", "status": "pending", "priority": "low"},
			{"content": "旧任务3", "status": "pending", "priority": "low"},
		},
	})
	res := todo.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// 第二次写入 1 项，应完全替换
	args = mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"content": "新任务", "status": "in_progress", "priority": "high"},
		},
	})
	res = todo.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if strings.Contains(res.Content, "旧任务") {
		t.Errorf("old items should be replaced: %s", res.Content)
	}
	if !strings.Contains(res.Content, "新任务") {
		t.Errorf("missing 新任务: %s", res.Content)
	}
}

func TestTodo_invalidJSON(t *testing.T) {
	todo := NewTodo()
	res := todo.Run(context.Background(), json.RawMessage(`{bad json`))
	if !res.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestTodo_concurrentSafe(t *testing.T) {
	todo := NewTodo()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			args := mustJSON(t, map[string]any{
				"todos": []map[string]string{
					{"content": "并发任务", "status": "pending", "priority": "low"},
				},
			})
			_ = todo.Run(context.Background(), args)
		}(i)
	}
	wg.Wait()
	// 不 panic 即通过
}

func TestTodo_metaHasCount(t *testing.T) {
	todo := NewTodo()
	args := mustJSON(t, map[string]any{
		"todos": []map[string]string{
			{"content": "任务", "status": "pending", "priority": "high"},
			{"content": "任务2", "status": "completed", "priority": "medium"},
		},
	})
	res := todo.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if res.Meta == nil {
		t.Fatal("Meta should not be nil")
	}
	count, ok := res.Meta["count"]
	if !ok {
		t.Fatal("Meta should have count")
	}
	if count != 2 {
		t.Errorf("count = %v, want 2", count)
	}
}
