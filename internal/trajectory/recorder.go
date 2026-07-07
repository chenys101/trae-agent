package trajectory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/paths"
)

// Record 单条轨迹记录，对应一个 agent 事件。
type Record struct {
	Timestamp  string          `json:"ts"`
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolArgs   string          `json:"tool_args,omitempty"`
	ToolResult json.RawMessage `json:"tool_result,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
	Usage      *llm.Usage      `json:"usage,omitempty"`
}

// Recorder JSONL 格式轨迹记录器。
type Recorder struct {
	mu  sync.Mutex
	f   *os.File
	enc *json.Encoder
}

// NewRecorder 打开（或创建）轨迹文件，append 模式。
func NewRecorder(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create trajectory dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open trajectory file: %w", err)
	}
	return &Recorder{f: f, enc: json.NewEncoder(f)}, nil
}

// RecordEvent 将 agent 事件写入轨迹文件。
func (r *Recorder) RecordEvent(ev agent.Event) error {
	if r == nil || r.f == nil {
		return nil
	}
	rec := r.eventToRecord(ev)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enc.Encode(rec)
}

// Close 关闭轨迹文件。幂等。
func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

func (r *Recorder) eventToRecord(ev agent.Event) Record {
	rec := Record{Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
	switch e := ev.(type) {
	case agent.TextEvent:
		rec.Type = "text"
		rec.Text = e.Content
	case agent.ToolCallEvent:
		rec.Type = "tool_call"
		rec.ToolName = e.Name
		rec.ToolArgs = e.Args
	case agent.ToolResultEvent:
		rec.Type = "tool_result"
		rec.ToolName = e.Name
		if b, err := json.Marshal(e.Result.Content); err == nil {
			rec.ToolResult = b
		}
		rec.IsError = e.Result.IsError
	case agent.DoneEvent:
		rec.Type = "done"
		u := e.Usage
		rec.Usage = &u
	default:
		rec.Type = "unknown"
	}
	return rec
}

// DefaultPath 返回默认轨迹文件路径（用户主目录下 .trae/trajectories/）。
func DefaultPath(sessionID string) string {
	p, err := paths.UnderTrae("trajectories", sessionID+".jsonl")
	if err != nil {
		// 极端情况：无法创建目录，回退到当前目录
		return sessionID + ".jsonl"
	}
	return p
}
