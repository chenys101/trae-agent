package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"time"
)

type Bash struct {
	defaultTimeout time.Duration
}

func NewBash(defaultTimeout time.Duration) *Bash {
	if defaultTimeout <= 0 {
		defaultTimeout = 120 * time.Second
	}
	return &Bash{defaultTimeout: defaultTimeout}
}

func (Bash) Name() string        { return "bash" }
func (Bash) Description() string { return "Execute a bash command. Returns combined stdout+stderr." }

func (Bash) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {"type": "string", "description": "bash command to execute"},
    "workdir": {"type": "string", "description": "working directory, default cwd"},
    "timeout_ms": {"type": "integer", "description": "timeout in milliseconds"}
  },
  "required": ["command"]
}`)
}

type bashArgs struct {
	Command   string `json:"command"`
	Workdir   string `json:"workdir"`
	TimeoutMs int    `json:"timeout_ms"`
}

func (b *Bash) Run(ctx context.Context, args json.RawMessage) Result {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.Command == "" {
		return ErrorResult("command is required")
	}

	timeout := b.defaultTimeout
	if a.TimeoutMs > 0 {
		timeout = time.Duration(a.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", a.Command)
	if a.Workdir != "" {
		cmd.Dir = a.Workdir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrorResult("timeout after %s\n%s", timeout, output)
		}
		return ErrorResult("exit: %v\n%s", err, output)
	}
	return Result{Content: output}
}
