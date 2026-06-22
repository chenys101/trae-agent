package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Store 权限规则持久化存储。
type Store struct {
	path string
}

// NewStore 创建存储实例，路径为 ~/.trae/permissions.json。
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".trae")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "permissions.json")}, nil
}

// NewStoreWithPath 用指定路径创建存储（供测试）。
func NewStoreWithPath(path string) *Store {
	return &Store{path: path}
}

// Load 加载用户规则到策略中。文件不存在时返回空策略（含内置规则）。
func (s *Store) Load() (*DefaultPolicy, error) {
	p := NewPolicy()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return nil, err
	}
	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	for _, r := range rules {
		p.AddUserRule(r)
	}
	return p, nil
}

// Save 持久化用户规则（仅用户规则，不含内置规则）。
func (s *Store) Save(p *DefaultPolicy) error {
	rules := p.UserRules()
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	// 原子写入：先写临时文件再 rename
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
