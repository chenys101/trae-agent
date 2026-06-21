package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBash_simpleCommand(t *testing.T) {
	r := NewBash(10 * time.Second)
	args, _ := json.Marshal(map[string]string{"command": "echo hello"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "hello") {
		t.Errorf("missing hello: %s", res.Content)
	}
}

func TestBash_exitCode(t *testing.T) {
	r := NewBash(10 * time.Second)
	args, _ := json.Marshal(map[string]string{"command": "exit 3"})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for non-zero exit")
	}
}

func TestBash_timeout(t *testing.T) {
	r := NewBash(100 * time.Millisecond)
	args, _ := json.Marshal(map[string]string{"command": "sleep 5"})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected timeout error")
	}
}

func TestBash_contextCancel(t *testing.T) {
	r := NewBash(10 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	args, _ := json.Marshal(map[string]string{"command": "sleep 5"})
	res := r.Run(ctx, args)
	if !res.IsError {
		t.Error("expected cancel error")
	}
}

func TestBash_workdir(t *testing.T) {
	r := NewBash(10 * time.Second)
	args, _ := json.Marshal(map[string]string{"command": "pwd", "workdir": "/tmp"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "/tmp") {
		t.Errorf("pwd should contain /tmp: %s", res.Content)
	}
}

func TestBash_outputTruncation(t *testing.T) {
	r := NewBash(10 * time.Second)
	// 输出 2MB，超过 1MB 上限应被截断并附加提示
	args, _ := json.Marshal(map[string]string{"command": "yes | head -c 2000000"})
	res := r.Run(context.Background(), args)
	if !strings.Contains(res.Content, "truncated") {
		t.Errorf("expected truncation notice, got content length %d", len(res.Content))
	}
}

func TestBash_timeoutMsTooLarge(t *testing.T) {
	r := NewBash(10 * time.Second)
	// timeout_ms 超过 600s 上限应被拒绝
	args, _ := json.Marshal(map[string]any{"command": "echo hi", "timeout_ms": 600001})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for timeout_ms too large")
	}
	if !strings.Contains(res.Content, "timeout_ms") && !strings.Contains(res.Content, "large") {
		t.Errorf("unexpected error message: %s", res.Content)
	}
}

func TestBash_envFiltering(t *testing.T) {
	// 设置含 KEY 的敏感环境变量，应被 filterEnv 过滤掉
	t.Setenv("MY_API_KEY", "TRAETEST_secret_xyz123")
	// 设置一个普通变量，验证 env 命令确实执行了
	t.Setenv("TRAETEST_NORMAL_VAR", "present_in_env")
	r := NewBash(10 * time.Second)
	args, _ := json.Marshal(map[string]string{"command": "env"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	// 敏感变量应被过滤，不出现在子进程环境中
	if strings.Contains(res.Content, "TRAETEST_secret_xyz123") {
		t.Errorf("secret leaked to child process: %s", res.Content)
	}
	// 普通变量应正常传递
	if !strings.Contains(res.Content, "present_in_env") {
		t.Errorf("normal env var missing from child process")
	}
}
