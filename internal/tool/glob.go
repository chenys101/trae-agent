package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Glob struct{}

func NewGlob() *Glob { return &Glob{} }

func (Glob) Name() string        { return "glob" }
func (Glob) Description() string { return "Find files matching a glob pattern (supports **)." }

func (Glob) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {"type": "string", "description": "glob pattern, e.g. **/*.go"},
    "path": {"type": "string", "description": "directory to search, default cwd"}
  },
  "required": ["pattern"]
}`)
}

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (Glob) Run(ctx context.Context, args json.RawMessage) Result {
	var a globArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.Pattern == "" {
		return ErrorResult("pattern is required")
	}
	root := a.Path
	if root == "" {
		root, _ = os.Getwd()
	}

	var matches []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过无法访问的路径（权限等）
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		ok, err := matchGlob(a.Pattern, rel)
		if err != nil {
			return err
		}
		if ok {
			matches = append(matches, p)
		}
		return nil
	})
	if err != nil {
		return ErrorResult("walk: %v", err)
	}

	return Result{Content: strings.Join(matches, "\n")}
}

// matchGlob 支持 ** 通配。
func matchGlob(pattern, name string) (bool, error) {
	// 统一用 ToSlash 后按 "/" 分割，避免 Windows 上 pattern 与 name 分隔符不一致。
	parts := strings.Split(filepath.ToSlash(pattern), "/")
	nameParts := strings.Split(filepath.ToSlash(name), "/")
	// 限制 ** 数量，避免多个 ** 导致指数级回溯（ReDoS 风险）。
	doubleStar := 0
	for _, p := range parts {
		if p == "**" {
			doubleStar++
		}
	}
	if doubleStar > 5 {
		return false, nil
	}
	return matchSegments(parts, nameParts)
}

func matchSegments(pat, name []string) (bool, error) {
	if len(pat) == 0 {
		return len(name) == 0, nil
	}
	if pat[0] == "**" {
		for i := 0; i <= len(name); i++ {
			ok, err := matchSegments(pat[1:], name[i:])
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	if len(name) == 0 {
		return false, nil
	}
	ok, err := filepath.Match(pat[0], name[0])
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return matchSegments(pat[1:], name[1:])
}
