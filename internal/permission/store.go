package permission

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bytedance/trae-agent/internal/errors"
	"github.com/bytedance/trae-agent/internal/paths"
	"github.com/bytedance/trae-agent/internal/util"
)

// Store 权限规则持久化存储。
type Store struct {
	path string
}

// NewStore 创建存储实例，路径为用户主目录下 .trae/permissions.json。
func NewStore() (*Store, error) {
	dir, err := paths.TraeDir()
	if err != nil {
		// .trae 目录创建失败
		return nil, errors.Wrap(err, errors.CodeConfigLoad, "create .trae dir")
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
		// 文件不存在时返回空策略（含内置规则），保留 os.IsNotExist 判断
		if os.IsNotExist(err) {
			return p, nil
		}
		return nil, err
	}
	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, errors.Wrap(err, errors.CodeConfigLoad, "unmarshal permissions")
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
		return errors.Wrap(err, errors.CodeConfigLoad, "marshal permissions")
	}
	// 原子写入：先写临时文件再 rename，Windows 上 rename 失败时回退到直接写入
	if err := util.AtomicWrite(s.path, data, 0o600); err != nil {
		return errors.Wrap(err, errors.CodeConfigLoad, "write permissions")
	}
	return nil
}
