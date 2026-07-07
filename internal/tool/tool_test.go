package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echo back the input" }
func (echoTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`)
}
func (echoTool) Run(ctx context.Context, args json.RawMessage) Result {
	var v struct {
		Msg string `json:"msg"`
	}
	json.Unmarshal(args, &v)
	return Result{Content: v.Msg}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry(echoTool{})
	got, ok := r.Get("echo")
	if !ok {
		t.Fatal("echo not found")
	}
	res := got.Run(context.Background(), json.RawMessage(`{"msg":"hi"}`))
	if res.Content != "hi" {
		t.Errorf("content = %q, want hi", res.Content)
	}
}

func TestRegistry_Get_notFound(t *testing.T) {
	r := NewRegistry(echoTool{})
	_, ok := r.Get("nope")
	if ok {
		t.Error("expected not found")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry(echoTool{})
	tools := r.List()
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	if tools[0].Name() != "echo" {
		t.Errorf("name = %q, want echo", tools[0].Name())
	}
}

func TestErrorResult(t *testing.T) {
	r := ErrorResult("bad: %s", "x")
	if !r.IsError {
		t.Error("IsError should be true")
	}
	if r.Content != "bad: x" {
		t.Errorf("content = %q, want 'bad: x'", r.Content)
	}
}
