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
