package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Grep struct{}

func NewGrep() *Grep { return &Grep{} }

func (Grep) Name() string { return "grep" }
func (Grep) Description() string {
	return "Search file contents with regex. Returns matches with file:line:content."
}

func (Grep) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {"type": "string", "description": "regex pattern"},
    "path": {"type": "string", "description": "directory or file to search"}
  },
  "required": ["pattern"]
}`)
}

type grepArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (Grep) Run(ctx context.Context, args json.RawMessage) Result {
	var a grepArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.Pattern == "" {
		return ErrorResult("pattern is required")
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return ErrorResult("compile regex: %v", err)
	}
	root := a.Path
	if root == "" {
		root, _ = os.Getwd()
	}

	var b strings.Builder
	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过无法访问的路径（权限等）
		}
		// 跳过非普通文件（目录、设备、管道、socket 等），避免读取异常。
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		// 增大 buffer，避免长行被 scanner 默认 64KB 上限丢弃。
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if re.MatchString(scanner.Text()) {
				fmt.Fprintf(&b, "%s:%d:%s\n", p, lineNum, scanner.Text())
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ErrorResult("walk: %v", err)
	}
	return Result{Content: b.String()}
}
