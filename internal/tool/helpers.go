package tool

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/bytedance/trae-agent/internal/consts"
)

// parseArgs 反序列化工具参数 JSON，失败时返回统一的 ErrorResult。
// 消除各工具中重复的 json.Unmarshal + 错误处理。
func parseArgs(args json.RawMessage, dst any) *Result {
	if err := json.Unmarshal(args, dst); err != nil {
		r := ErrorResult("parse args: %v", err)
		return &r
	}
	return nil
}

// readFileWithLimit 读取文件并检查大小限制。
// 超过 consts.MaxFileSize 时返回错误。消除 read.go 和 edit.go 的重复逻辑。
func readFileWithLimit(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > consts.MaxFileSize {
		return nil, fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), consts.MaxFileSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return data, nil
}
