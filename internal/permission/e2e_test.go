package permission

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestE2E_fullFlowAllowAlways 验证完整流程：
// load policy → check → ask → persist allow-always → recheck（now allowed）。
func TestE2E_fullFlowAllowAlways(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	store := NewStoreWithPath(path)

	// 1. 加载策略（文件不存在，返回仅含内置规则的策略）
	p, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// 2. 检查 git push（内置规则返回 Ask）
	args := json.RawMessage(`{"command":"git push origin main"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAsk {
		t.Fatalf("initial check action = %v, want Ask", d.Action)
	}

	// 3. 用户通过 asker 选择 AllowAlways
	asker := &CallbackAsker{
		Func: func(ctx context.Context, tool, args, reason string) Action {
			return ActionAllowAlways
		},
	}
	decision := asker.Ask(context.Background(), "bash", string(args), d.Reason)
	if decision != ActionAllowAlways {
		t.Fatalf("asker returned %v, want AllowAlways", decision)
	}

	// 4. 持久化 allow-always 规则
	p.AddUserRule(Rule{
		Tool:   "bash",
		Args:   []string{"git push"},
		Action: ActionAllow,
		Desc:   "user allowed git push",
	})
	if err := store.Save(p); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 5. 重新检查，应直接 Allow（用户规则覆盖内置 Ask）
	d = p.Check("bash", args)
	if d.Action != ActionAllow {
		t.Errorf("after persist: action = %v, want Allow", d.Action)
	}

	// 6. 重新加载，验证规则持久化
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	d = loaded.Check("bash", args)
	if d.Action != ActionAllow {
		t.Errorf("after reload: action = %v, want Allow", d.Action)
	}
}

// TestE2E_denyAlwaysPersisted 验证 Deny 规则持久化后重新加载仍生效。
func TestE2E_denyAlwaysPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	store := NewStoreWithPath(path)

	p, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// 添加用户 Deny 规则：禁止 ls
	p.AddUserRule(Rule{
		Tool:   "bash",
		Args:   []string{"ls"},
		Action: ActionDeny,
		Desc:   "user denied ls",
	})
	if err := store.Save(p); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 重新加载
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}

	// ls 应被 Deny
	args := json.RawMessage(`{"command":"ls -la"}`)
	d := loaded.Check("bash", args)
	if d.Action != ActionDeny {
		t.Errorf("after reload: action = %v, want Deny", d.Action)
	}
}

// TestE2E_clearRulesResetsToBuiltins 验证清除规则后回到仅内置规则状态。
func TestE2E_clearRulesResetsToBuiltins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	store := NewStoreWithPath(path)

	p, err := store.Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	// 添加用户规则
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionDeny, Desc: "deny ls"})
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"cat"}, Action: ActionDeny, Desc: "deny cat"})

	// 验证用户规则生效
	args := json.RawMessage(`{"command":"ls -la"}`)
	if d := p.Check("bash", args); d.Action != ActionDeny {
		t.Fatalf("before clear: action = %v, want Deny", d.Action)
	}

	// 清除用户规则
	p.ClearUserRules()

	// 保存（应保存空用户规则）
	if err := store.Save(p); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 重新加载
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}

	// ls 应回到默认 Allow
	if d := loaded.Check("bash", args); d.Action != ActionAllow {
		t.Errorf("after clear+reload: action = %v, want Allow", d.Action)
	}
	// 内置规则仍生效
	rmArgs := json.RawMessage(`{"command":"rm -rf /"}`)
	if d := loaded.Check("bash", rmArgs); d.Action != ActionDeny {
		t.Errorf("builtin after clear: action = %v, want Deny", d.Action)
	}
}

// TestE2E_builtinsAlwaysPresent 验证无论用户规则如何，内置规则始终存在。
func TestE2E_builtinsAlwaysPresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	store := NewStoreWithPath(path)

	// 第一次加载（无文件）
	p1, err := store.Load()
	if err != nil {
		t.Fatalf("first Load error: %v", err)
	}
	p1.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionAllow, Desc: "allow ls"})
	if err := store.Save(p1); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 第二次加载
	p2, err := store.Load()
	if err != nil {
		t.Fatalf("second Load error: %v", err)
	}

	// 内置危险命令仍被 Deny
	cases := []struct {
		name string
		args string
	}{
		{"rm -rf /", `{"command":"rm -rf /"}`},
		{"mkfs", `{"command":"mkfs.ext4 /dev/sda"}`},
		{"dd if=", `{"command":"dd if=/dev/zero of=/tmp/x"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := p2.Check("bash", json.RawMessage(c.args))
			if d.Action != ActionDeny {
				t.Errorf("builtin %s: action = %v, want Deny", c.name, d.Action)
			}
		})
	}
}

// TestE2E_userRulePersistsAcrossReloads 验证用户规则跨多次加载仍存在。
func TestE2E_userRulePersistsAcrossReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.json")
	store := NewStoreWithPath(path)

	// 第一次：添加并保存
	p1, _ := store.Load()
	p1.AddUserRule(Rule{Tool: "bash", Args: []string{"make"}, Action: ActionAsk, Desc: "ask make"})
	if err := store.Save(p1); err != nil {
		t.Fatalf("first Save error: %v", err)
	}

	// 第二次：加载、添加另一条、保存
	p2, _ := store.Load()
	if len(p2.UserRules()) != 1 {
		t.Fatalf("after first reload: expected 1 user rule, got %d", len(p2.UserRules()))
	}
	p2.AddUserRule(Rule{Tool: "bash", Args: []string{"cargo"}, Action: ActionAsk, Desc: "ask cargo"})
	if err := store.Save(p2); err != nil {
		t.Fatalf("second Save error: %v", err)
	}

	// 第三次：加载，应有 2 条用户规则
	p3, _ := store.Load()
	rules := p3.UserRules()
	if len(rules) != 2 {
		t.Fatalf("after second reload: expected 2 user rules, got %d", len(rules))
	}

	// 两条规则都应生效
	makeArgs := json.RawMessage(`{"command":"make build"}`)
	if d := p3.Check("bash", makeArgs); d.Action != ActionAsk {
		t.Errorf("make rule: action = %v, want Ask", d.Action)
	}
	cargoArgs := json.RawMessage(`{"command":"cargo build"}`)
	if d := p3.Check("bash", cargoArgs); d.Action != ActionAsk {
		t.Errorf("cargo rule: action = %v, want Ask", d.Action)
	}
}

// TestE2E_askerDenyPreventsExecution 验证 asker 返回 Deny 时模拟阻止执行。
// 这个测试模拟 dispatcher 的行为：policy 返回 Ask → asker 返回 Deny → 不执行。
func TestE2E_askerDenyPreventsExecution(t *testing.T) {
	p := NewPolicy()
	asker := &CallbackAsker{
		Func: func(ctx context.Context, tool, args, reason string) Action {
			return ActionDeny // 用户拒绝
		},
	}

	args := json.RawMessage(`{"command":"git push origin main"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAsk {
		t.Fatalf("policy action = %v, want Ask", d.Action)
	}

	// 模拟 dispatcher 的 ask 流程
	decision := asker.Ask(context.Background(), "bash", string(args), d.Reason)
	if decision != ActionDeny {
		t.Fatalf("asker decision = %v, want Deny", decision)
	}
	// dispatcher 应返回 error（不执行工具）
	// 这里只验证决策逻辑，实际执行由 dispatcher 测试覆盖
}

// TestE2E_askerAllowProceedsExecution 验证 asker 返回 Allow 时模拟允许执行。
func TestE2E_askerAllowProceedsExecution(t *testing.T) {
	p := NewPolicy()
	asker := &CallbackAsker{
		Func: func(ctx context.Context, tool, args, reason string) Action {
			return ActionAllow // 用户允许
		},
	}

	args := json.RawMessage(`{"command":"git push origin main"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAsk {
		t.Fatalf("policy action = %v, want Ask", d.Action)
	}

	decision := asker.Ask(context.Background(), "bash", string(args), d.Reason)
	if decision != ActionAllow {
		t.Fatalf("asker decision = %v, want Allow", decision)
	}
	// dispatcher 应执行工具
}

// TestE2E_autoAskerDenyInHeadless 验证 headless 模式下 AutoAsker 默认 Deny。
func TestE2E_autoAskerDenyInHeadless(t *testing.T) {
	p := NewPolicy()
	// headless 模式默认拒绝需要确认的操作
	asker := &AutoAsker{Default: ActionDeny}

	args := json.RawMessage(`{"command":"git push origin main"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAsk {
		t.Fatalf("policy action = %v, want Ask", d.Action)
	}

	decision := asker.Ask(context.Background(), "bash", string(args), d.Reason)
	if decision != ActionDeny {
		t.Errorf("headless AutoAsker decision = %v, want Deny", decision)
	}
}

// TestE2E_storePathIsolation 验证不同 store 路径互不干扰。
func TestE2E_storePathIsolation(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "p1.json")
	path2 := filepath.Join(dir, "p2.json")

	s1 := NewStoreWithPath(path1)
	s2 := NewStoreWithPath(path2)

	// s1 保存 deny ls
	p1, _ := s1.Load()
	p1.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionDeny, Desc: "deny ls"})
	if err := s1.Save(p1); err != nil {
		t.Fatalf("s1.Save error: %v", err)
	}

	// s2 保存 allow ls
	p2, _ := s2.Load()
	p2.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionAllow, Desc: "allow ls"})
	if err := s2.Save(p2); err != nil {
		t.Fatalf("s2.Save error: %v", err)
	}

	// 重新加载，验证隔离
	loaded1, _ := s1.Load()
	loaded2, _ := s2.Load()

	args := json.RawMessage(`{"command":"ls -la"}`)
	if d := loaded1.Check("bash", args); d.Action != ActionDeny {
		t.Errorf("s1: action = %v, want Deny", d.Action)
	}
	if d := loaded2.Check("bash", args); d.Action != ActionAllow {
		t.Errorf("s2: action = %v, want Allow", d.Action)
	}
}
