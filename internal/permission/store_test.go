package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestStore_loadNonexistentFile 验证加载不存在的文件返回仅含内置规则的策略。
func TestStore_loadNonexistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	s := NewStoreWithPath(path)
	p, err := s.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	// 内置规则应存在
	args := json.RawMessage(`{"command":"rm -rf /"}`)
	if d := p.Check("bash", args); d.Action != ActionDeny {
		t.Errorf("builtin rule missing: action = %v, want Deny", d.Action)
	}
	// 用户规则应为空
	if rules := p.UserRules(); len(rules) != 0 {
		t.Errorf("expected 0 user rules, got %d", len(rules))
	}
}

// TestStore_saveThenLoadPreservesUserRules 验证保存后加载能恢复用户规则。
func TestStore_saveThenLoadPreservesUserRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	s := NewStoreWithPath(path)

	p := NewPolicy()
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionDeny, Desc: "deny ls"})
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"cat"}, Action: ActionAllow, Desc: "allow cat"})

	if err := s.Save(p); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	rules := loaded.UserRules()
	if len(rules) != 2 {
		t.Fatalf("expected 2 user rules, got %d", len(rules))
	}
	if rules[0].Desc != "deny ls" || rules[0].Action != ActionDeny {
		t.Errorf("rule[0] = %+v", rules[0])
	}
	if rules[1].Desc != "allow cat" || rules[1].Action != ActionAllow {
		t.Errorf("rule[1] = %+v", rules[1])
	}
}

// TestStore_saveOnlyPersistsUserRules 验证 Save 只持久化用户规则，不含内置规则。
func TestStore_saveOnlyPersistsUserRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	s := NewStoreWithPath(path)

	p := NewPolicy()
	// 不添加任何用户规则，只保存
	if err := s.Save(p); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	// 直接读取文件验证内容
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	// 应该是空数组（无用户规则）
	var rules []Rule
	if err := json.Unmarshal(data, &rules); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules in file (no builtins), got %d", len(rules))
	}
}

// TestStore_newStoreWithPath 验证 NewStoreWithPath 创建的 store 路径正确。
func TestStore_newStoreWithPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.json")
	s := NewStoreWithPath(path)
	if s.path != path {
		t.Errorf("path = %q, want %q", s.path, path)
	}
}

// TestStore_saveOverwritesExisting 验证 Save 覆盖已有文件。
func TestStore_saveOverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	s := NewStoreWithPath(path)

	// 第一次保存 1 条规则
	p1 := NewPolicy()
	p1.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionDeny, Desc: "first"})
	if err := s.Save(p1); err != nil {
		t.Fatalf("first Save error: %v", err)
	}

	// 第二次保存 2 条规则（覆盖）
	p2 := NewPolicy()
	p2.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionDeny, Desc: "second"})
	p2.AddUserRule(Rule{Tool: "bash", Args: []string{"cat"}, Action: ActionAllow, Desc: "third"})
	if err := s.Save(p2); err != nil {
		t.Fatalf("second Save error: %v", err)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	rules := loaded.UserRules()
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules after overwrite, got %d", len(rules))
	}
	if rules[0].Desc != "second" {
		t.Errorf("rule[0].Desc = %q, want 'second'", rules[0].Desc)
	}
}

// TestStore_loadInvalidJSON 验证加载损坏的 JSON 返回错误。
func TestStore_loadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	if err := os.WriteFile(path, []byte("{invalid json}"), 0o600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	s := NewStoreWithPath(path)
	_, err := s.Load()
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestStore_saveEmptyUserRules 验证保存空用户规则生成合法文件。
func TestStore_saveEmptyUserRules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	s := NewStoreWithPath(path)
	p := NewPolicy()
	if err := s.Save(p); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	// 文件应存在
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}
