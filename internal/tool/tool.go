package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Result 工具执行结果。
type Result struct {
	Content string         // 文本结果（回灌给 LLM）
	IsError bool           // 是否为错误
	Meta    map[string]any // 元信息（行号、文件路径等，可选）
}

// maxFileSize 文件读写大小上限（10MB），超过拒绝以避免 OOM。
const maxFileSize = 10 << 20

// Tool 工具接口。
type Tool interface {
	Name() string
	Description() string
	// Schema 返回 JSON schema 描述参数（暴露给 LLM）。
	Schema() json.RawMessage
	// Run 执行工具，args 是 LLM 传入的 JSON 参数。
	Run(ctx context.Context, args json.RawMessage) Result
}

// Registry 工具注册表。
type Registry struct {
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
	return r
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	// 按 Name 排序，保证返回顺序确定（map 遍历顺序不确定）。
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}

// ErrorResult 构造错误结果的快捷函数。
func ErrorResult(format string, args ...any) Result {
	return Result{Content: fmt.Sprintf(format, args...), IsError: true}
}
