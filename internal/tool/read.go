package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Read struct{}

func NewRead() *Read { return &Read{} }

func (Read) Name() string        { return "read" }
func (Read) Description() string { return "Read a file with line numbers. Supports offset and limit." }

func (Read) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "absolute path to the file"},
    "offset": {"type": "integer", "description": "1-based line number to start from"},
    "limit": {"type": "integer", "description": "max number of lines to read"}
  },
  "required": ["file_path"]
}`)
}

type readArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

func (Read) Run(ctx context.Context, args json.RawMessage) Result {
	var a readArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.FilePath == "" {
		return ErrorResult("file_path is required")
	}

	data, err := os.ReadFile(a.FilePath)
	if err != nil {
		return ErrorResult("read file: %v", err)
	}
	if len(data) == 0 {
		return Result{Content: ""}
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	start := 1
	if a.Offset > 0 {
		start = a.Offset
	}
	end := len(lines)
	if a.Limit > 0 && start-1+a.Limit < end {
		end = start - 1 + a.Limit
	}
	if start > len(lines) {
		return Result{Content: ""}
	}
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}

	maxWidth := len(fmt.Sprintf("%d", end))
	var b strings.Builder
	for i := start - 1; i < end; i++ {
		fmt.Fprintf(&b, "%*d→%s\n", maxWidth, i+1, lines[i])
	}
	return Result{Content: b.String()}
}
