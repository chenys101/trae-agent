// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package tool

import (
	"fmt"
	"sort"
	"sync"
)

// Registry 管理工具的注册和实例化。
// 使用工厂模式，每个工具注册一个工厂函数，按需创建工具实例。
type Registry struct {
	mu       sync.RWMutex
	factories map[string]func() Tool
}

// Register 注册一个工具工厂函数。
// 名称会通过 normalizeName 进行标准化处理。
func (r *Registry) Register(name string, factory func() Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	normalized := normalizeName(name)
	r.factories[normalized] = factory
}

// Get 根据名称获取工具实例。
// 如果工具不存在，返回错误。
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	normalized := normalizeName(name)
	factory, ok := r.factories[normalized]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return factory(), nil
}

// List 返回所有已注册工具的名称列表，按字母序排列。
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// NewDefaultRegistry 创建并注册所有内置工具的 Registry。
func NewDefaultRegistry() *Registry {
	r := &Registry{
		factories: make(map[string]func() Tool),
	}
	r.Register("bash", NewBashTool)
	r.Register("read_file", NewReadFileTool)
	r.Register("grep_search", NewGrepSearchTool)
	r.Register("glob_search", NewGlobSearchTool)
	r.Register("edit", NewEditTool)
	r.Register("sequential_thinking", NewSequentialThinkingTool)
	r.Register("task_done", NewTaskDoneTool)
	return r
}
