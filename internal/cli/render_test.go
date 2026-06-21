package cli

import (
	"bytes"
	"testing"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

func TestRenderer_textEvent(t *testing.T) {
	r := NewRenderer()
	events := make(chan agent.Event, 1)
	events <- agent.TextEvent{Content: "hello"}
	close(events)

	var buf bytes.Buffer
	usage, err := r.Render(events, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello\n" {
		t.Errorf("output = %q, want 'hello\\n'", buf.String())
	}
	if usage.InputTokens != 0 {
		t.Errorf("usage should be zero: %+v", usage)
	}
}

func TestRenderer_toolCallEvent(t *testing.T) {
	r := NewRenderer()
	events := make(chan agent.Event, 2)
	events <- agent.ToolCallEvent{Name: "read", Args: `{"file_path":"/tmp/a"}`}
	events <- agent.DoneEvent{Usage: llm.Usage{InputTokens: 5, OutputTokens: 3}}
	close(events)

	var buf bytes.Buffer
	usage, _ := r.Render(events, &buf)
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("read")) {
		t.Errorf("missing tool name: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("/tmp/a")) {
		t.Errorf("missing tool args: %s", out)
	}
	if usage.InputTokens != 5 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestRenderer_toolResultEvent(t *testing.T) {
	r := NewRenderer()
	events := make(chan agent.Event, 2)
	events <- agent.ToolResultEvent{
		Name:   "read",
		Result: tool.Result{Content: "file content here"},
	}
	close(events)

	var buf bytes.Buffer
	r.Render(events, &buf)
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("file content here")) {
		t.Errorf("missing result content: %s", out)
	}
}

func TestRenderer_toolResultError(t *testing.T) {
	r := NewRenderer()
	events := make(chan agent.Event, 1)
	events <- agent.ToolResultEvent{
		Name:   "bash",
		Result: tool.Result{Content: "command failed", IsError: true},
	}
	close(events)

	var buf bytes.Buffer
	r.Render(events, &buf)
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("command failed")) {
		t.Errorf("missing error content: %s", out)
	}
}

func TestRenderer_truncation(t *testing.T) {
	r := NewRenderer()
	longContent := make([]byte, 600)
	for i := range longContent {
		longContent[i] = 'x'
	}
	events := make(chan agent.Event, 1)
	events <- agent.ToolResultEvent{
		Name:   "read",
		Result: tool.Result{Content: string(longContent)},
	}
	close(events)

	var buf bytes.Buffer
	r.Render(events, &buf)
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("truncated")) {
		t.Errorf("long content should be truncated: %s", out[:50])
	}
}
