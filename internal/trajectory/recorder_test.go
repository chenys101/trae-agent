package trajectory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

func readRecords(t *testing.T, path string) []Record {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trajectory: %v", err)
	}
	var records []Record
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		records = append(records, rec)
	}
	return records
}

func TestRecorder_textEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traj.jsonl")
	r, err := NewRecorder(path)
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	if err := r.RecordEvent(agent.TextEvent{Content: "hello"}); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}
	r.Close()
	records := readRecords(t, path)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Type != "text" || records[0].Text != "hello" {
		t.Errorf("unexpected record: %+v", records[0])
	}
}

func TestRecorder_toolCallAndResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traj.jsonl")
	r, _ := NewRecorder(path)
	r.RecordEvent(agent.ToolCallEvent{Name: "bash", Args: `{"command":"ls"}`})
	r.RecordEvent(agent.ToolResultEvent{
		Name:   "bash",
		Result: tool.Result{Content: "file1\nfile2"},
	})
	r.Close()
	records := readRecords(t, path)
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].Type != "tool_call" || records[0].ToolName != "bash" {
		t.Errorf("unexpected tool_call: %+v", records[0])
	}
	if records[1].Type != "tool_result" || string(records[1].ToolResult) != `"file1\nfile2"` {
		t.Errorf("unexpected tool_result: %+v", records[1])
	}
}

func TestRecorder_doneEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traj.jsonl")
	r, _ := NewRecorder(path)
	r.RecordEvent(agent.DoneEvent{Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}})
	r.Close()
	records := readRecords(t, path)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Type != "done" {
		t.Errorf("expected done, got %s", records[0].Type)
	}
	if records[0].Usage == nil || records[0].Usage.InputTokens != 10 {
		t.Errorf("unexpected usage: %+v", records[0].Usage)
	}
}

func TestRecorder_append(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traj.jsonl")
	r1, _ := NewRecorder(path)
	r1.RecordEvent(agent.TextEvent{Content: "first"})
	r1.Close()
	r2, _ := NewRecorder(path)
	r2.RecordEvent(agent.TextEvent{Content: "second"})
	r2.Close()
	records := readRecords(t, path)
	if len(records) != 2 {
		t.Fatalf("expected 2 records (append), got %d", len(records))
	}
}

func TestRecorder_closeIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traj.jsonl")
	r, _ := NewRecorder(path)
	if err := r.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestDefaultPath(t *testing.T) {
	p := DefaultPath("session-123")
	if !strings.HasSuffix(p, "trajectories/session-123.jsonl") {
		t.Errorf("unexpected path: %s", p)
	}
}
