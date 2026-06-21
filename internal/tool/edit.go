package tool

import (
	"context"
	"encoding/json"
	"os"
	"strings"
)

type Edit struct{}

func NewEdit() *Edit { return &Edit{} }

func (Edit) Name() string        { return "edit" }
func (Edit) Description() string { return "Replace a unique string in a file. Errors if old_string is not found or not unique (unless replace_all)." }

func (Edit) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string"},
    "old_string": {"type": "string", "description": "must be unique unless replace_all"},
    "new_string": {"type": "string", "description": "replacement text; can be empty to delete old_string"},
    "replace_all": {"type": "boolean", "description": "replace all occurrences"}
  },
  "required": ["file_path", "old_string", "new_string"]
}`)
}

type editArgs struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func (Edit) Run(ctx context.Context, args json.RawMessage) Result {
	var a editArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.FilePath == "" {
		return ErrorResult("file_path is required")
	}
	if a.OldString == "" {
		return ErrorResult("old_string is required")
	}

	// 先 stat 检查文件大小
	info, err := os.Stat(a.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult("file not found: %s", a.FilePath)
		}
		return ErrorResult("stat file: %v", err)
	}
	if info.Size() > maxFileSize {
		return ErrorResult("file too large (%d bytes, max %d)", info.Size(), maxFileSize)
	}

	data, err := os.ReadFile(a.FilePath)
	if err != nil {
		return ErrorResult("read file: %v", err)
	}
	content := string(data)

	if a.ReplaceAll {
		if !strings.Contains(content, a.OldString) {
			return ErrorResult("old_string not found")
		}
		newContent := strings.ReplaceAll(content, a.OldString, a.NewString)
		if err := atomicWrite(a.FilePath, []byte(newContent)); err != nil {
			return ErrorResult("write file: %v", err)
		}
		return Result{Content: "replaced all in " + a.FilePath}
	}

	count := strings.Count(content, a.OldString)
	if count == 0 {
		return ErrorResult("old_string not found in %s", a.FilePath)
	}
	if count > 1 {
		return ErrorResult("old_string appears %d times, must be unique (use replace_all)", count)
	}

	newContent := strings.Replace(content, a.OldString, a.NewString, 1)
	if err := atomicWrite(a.FilePath, []byte(newContent)); err != nil {
		return ErrorResult("write file: %v", err)
	}
	return Result{Content: "edited " + a.FilePath}
}

// atomicWrite 原子写入：先写临时文件再 rename，避免写入中途崩溃导致文件损坏。
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
