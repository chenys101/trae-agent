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

	// 先 stat 检查文件大小，避免读取超大文件导致 OOM
	info, err := os.Stat(a.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult("file not found: %s", a.FilePath)
		}
		return ErrorResult("stat file: %v", err)
	}
	if info.Size() > maxFileSize {
		return ErrorResult("file too large (%d bytes, max %d): %s", info.Size(), maxFileSize, a.FilePath)
	}

	data, err := os.ReadFile(a.FilePath)
	if err != nil {
		return ErrorResult("read file: %v", err)
	}
	if len(data) == 0 {
		return Result{Content: ""}
	}

	// 用 TrimSuffix 而非 TrimRight：只移除一个尾部换行符，保留文件末尾的空行。
	// TrimRight 会吞掉所有尾部 \n，"a\n\n" 会被压缩为 ["a"]，丢失末尾空行。
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
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
