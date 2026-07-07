package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bytedance/trae-agent/internal/util"
)

type Write struct{}

func NewWrite() *Write { return &Write{} }

func (Write) Name() string        { return "write" }
func (Write) Description() string { return "Write content to a file, creating parent dirs if needed. Overwrites existing content." }

func (Write) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "absolute path to the file"},
    "content": {"type": "string", "description": "content to write"}
  },
  "required": ["file_path", "content"]
}`)
}

type writeArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (Write) Run(ctx context.Context, args json.RawMessage) Result {
	var a writeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.FilePath == "" {
		return ErrorResult("file_path is required")
	}
	if len(a.Content) > maxFileSize {
		return ErrorResult("content too large (%d bytes, max %d)", len(a.Content), maxFileSize)
	}

	dir := filepath.Dir(a.FilePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ErrorResult("mkdir: %v", err)
	}
	// 原子写：先写临时文件再 rename，避免写入中途崩溃导致文件损坏
	if err := util.AtomicWrite(a.FilePath, []byte(a.Content), 0o644); err != nil {
		return ErrorResult("write file: %v", err)
	}
	return Result{Content: "wrote " + a.FilePath}
}
