package permission

import (
	"encoding/json"
	"testing"
)

// TestPolicy_rmRfRootDenied 验证 rm -rf / 被内置规则 Deny。
func TestPolicy_rmRfRootDenied(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":"rm -rf /"}`)
	d := p.Check("bash", args)
	if d.Action != ActionDeny {
		t.Errorf("rm -rf / action = %v, want Deny", d.Action)
	}
	if d.Reason == "" {
		t.Error("expected non-empty reason")
	}
}

// TestPolicy_rmRfFileAsk 验证 rm -rf somefile 触发 Ask（不命中 rm -rf /）。
func TestPolicy_rmRfFileAsk(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":"rm -rf somefile"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAsk {
		t.Errorf("rm -rf somefile action = %v, want Ask", d.Action)
	}
}

// TestPolicy_mkfsDenied 验证 mkfs 被 Deny。
func TestPolicy_mkfsDenied(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":"mkfs.ext4 /dev/sda1"}`)
	d := p.Check("bash", args)
	if d.Action != ActionDeny {
		t.Errorf("mkfs action = %v, want Deny", d.Action)
	}
}

// TestPolicy_ddIfDenied 验证 dd if= 被 Deny。
func TestPolicy_ddIfDenied(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":"dd if=/dev/zero of=/dev/sda bs=1M"}`)
	d := p.Check("bash", args)
	if d.Action != ActionDeny {
		t.Errorf("dd if= action = %v, want Deny", d.Action)
	}
}

// TestPolicy_gitPushAsk 验证 git push 触发 Ask。
func TestPolicy_gitPushAsk(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":"git push origin main"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAsk {
		t.Errorf("git push action = %v, want Ask", d.Action)
	}
}

// TestPolicy_lsAllowed 验证 ls 无规则匹配时默认 Allow。
func TestPolicy_lsAllowed(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":"ls -la"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAllow {
		t.Errorf("ls action = %v, want Allow", d.Action)
	}
}

// TestPolicy_userRuleAllowOverridesBuiltinAsk 验证用户 Allow 规则覆盖内置 Ask。
func TestPolicy_userRuleAllowOverridesBuiltinAsk(t *testing.T) {
	p := NewPolicy()
	// 用户规则：允许 git push
	p.AddUserRule(Rule{
		Tool:   "bash",
		Args:   []string{"git push"},
		Action: ActionAllow,
		Desc:   "user allow git push",
	})
	args := json.RawMessage(`{"command":"git push origin main"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAllow {
		t.Errorf("user allow override action = %v, want Allow", d.Action)
	}
	if d.Matched != "user allow git push" {
		t.Errorf("matched = %q, want 'user allow git push'", d.Matched)
	}
}

// TestPolicy_userRuleDenyOverridesDefaultAllow 验证用户 Deny 规则覆盖默认 Allow。
func TestPolicy_userRuleDenyOverridesDefaultAllow(t *testing.T) {
	p := NewPolicy()
	// 用户规则：禁止 ls 命令
	p.AddUserRule(Rule{
		Tool:   "bash",
		Args:   []string{"ls"},
		Action: ActionDeny,
		Desc:   "user deny ls",
	})
	args := json.RawMessage(`{"command":"ls -la"}`)
	d := p.Check("bash", args)
	if d.Action != ActionDeny {
		t.Errorf("user deny override action = %v, want Deny", d.Action)
	}
}

// TestPolicy_clearUserRulesKeepsBuiltins 验证 ClearUserRules 移除用户规则但保留内置规则。
func TestPolicy_clearUserRulesKeepsBuiltins(t *testing.T) {
	p := NewPolicy()
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionDeny, Desc: "user deny ls"})
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"cat"}, Action: ActionDeny, Desc: "user deny cat"})

	// 清除前：用户规则生效
	args := json.RawMessage(`{"command":"ls -la"}`)
	if d := p.Check("bash", args); d.Action != ActionDeny {
		t.Fatalf("before clear: action = %v, want Deny", d.Action)
	}

	p.ClearUserRules()

	// 清除后：用户规则失效，回到默认 Allow
	if d := p.Check("bash", args); d.Action != ActionAllow {
		t.Errorf("after clear: action = %v, want Allow", d.Action)
	}
	// 内置规则仍生效
	rmArgs := json.RawMessage(`{"command":"rm -rf /"}`)
	if d := p.Check("bash", rmArgs); d.Action != ActionDeny {
		t.Errorf("after clear builtin: action = %v, want Deny", d.Action)
	}
	// UserRules 应为空
	if rules := p.UserRules(); len(rules) != 0 {
		t.Errorf("after clear: UserRules len = %d, want 0", len(rules))
	}
}

// TestPolicy_extractKeyArgBash 验证 ExtractKeyArg 对 bash 提取 command 字段。
func TestPolicy_extractKeyArgBash(t *testing.T) {
	args := json.RawMessage(`{"command":"rm -rf /tmp"}`)
	got := ExtractKeyArg("bash", args)
	want := "rm -rf /tmp"
	if got != want {
		t.Errorf("ExtractKeyArg(bash) = %q, want %q", got, want)
	}
}

// TestPolicy_extractKeyArgNonBash 验证 ExtractKeyArg 对非 bash 工具返回原始 JSON。
func TestPolicy_extractKeyArgNonBash(t *testing.T) {
	args := json.RawMessage(`{"path":"/tmp"}`)
	got := ExtractKeyArg("read", args)
	want := `{"path":"/tmp"}`
	if got != want {
		t.Errorf("ExtractKeyArg(read) = %q, want %q", got, want)
	}
}

// TestPolicy_matchRuleWildcardTool 验证通配符 "*" 匹配所有工具。
func TestPolicy_matchRuleWildcardTool(t *testing.T) {
	r := Rule{Tool: "*", Args: nil, Action: ActionDeny, Desc: "deny all"}
	// 任意工具都应匹配
	if !matchRule(r, "bash", json.RawMessage(`{}`)) {
		t.Error("wildcard should match bash")
	}
	if !matchRule(r, "read", json.RawMessage(`{}`)) {
		t.Error("wildcard should match read")
	}
	if !matchRule(r, "anything", json.RawMessage(`{}`)) {
		t.Error("wildcard should match anything")
	}
}

// TestPolicy_matchRuleExactTool 验证精确工具名匹配。
func TestPolicy_matchRuleExactTool(t *testing.T) {
	r := Rule{Tool: "bash", Args: nil, Action: ActionDeny, Desc: "deny bash"}
	if !matchRule(r, "bash", json.RawMessage(`{}`)) {
		t.Error("exact tool should match bash")
	}
	if matchRule(r, "read", json.RawMessage(`{}`)) {
		t.Error("exact tool should not match read")
	}
}

// TestPolicy_matchRuleArgsAnd 验证多个 Args 子串为 AND 关系。
func TestPolicy_matchRuleArgsAnd(t *testing.T) {
	r := Rule{Tool: "bash", Args: []string{"rm", "-rf"}, Action: ActionDeny, Desc: "rm -rf"}
	// 两个子串都在 → 匹配
	if !matchRule(r, "bash", json.RawMessage(`{"command":"rm -rf /tmp"}`)) {
		t.Error("should match when both args present")
	}
	// 只有一个子串 → 不匹配
	if matchRule(r, "bash", json.RawMessage(`{"command":"rm /tmp"}`)) {
		t.Error("should not match when only one arg present")
	}
}

// TestPolicy_rulesReturnsCopy 验证 Rules 返回副本，修改不影响内部状态。
func TestPolicy_rulesReturnsCopy(t *testing.T) {
	p := NewPolicy()
	rules := p.Rules()
	if len(rules) == 0 {
		t.Fatal("expected non-empty rules")
	}
	// 修改副本
	rules[0].Desc = "modified"
	// 内部规则不应被影响
	again := p.Rules()
	if again[0].Desc == "modified" {
		t.Error("Rules should return a copy, internal state was modified")
	}
}

// TestPolicy_userRulesReturnsCopy 验证 UserRules 返回副本。
func TestPolicy_userRulesReturnsCopy(t *testing.T) {
	p := NewPolicy()
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionDeny, Desc: "original"})
	rules := p.UserRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 user rule, got %d", len(rules))
	}
	rules[0].Desc = "modified"
	again := p.UserRules()
	if again[0].Desc != "original" {
		t.Errorf("UserRules should return a copy, got %q", again[0].Desc)
	}
}

// TestPolicy_rmRfRootPrioritizedOverRmRf 验证内置规则中 rm -rf / 优先于 rm -rf。
// 内置规则从前往后匹配，rm -rf / 在前，rm -rf 在后，所以 rm -rf / 命中 Deny 而非 Ask。
func TestPolicy_rmRfRootPrioritizedOverRmRf(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":"rm -rf /"}`)
	d := p.Check("bash", args)
	if d.Action != ActionDeny {
		t.Errorf("rm -rf / should be Deny (prioritized over rm -rf Ask), got %v", d.Action)
	}
}

// TestPolicy_forkBombDenied 验证 fork bomb 被 Deny。
func TestPolicy_forkBombDenied(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":":(){ :|:& };:"}`)
	d := p.Check("bash", args)
	if d.Action != ActionDeny {
		t.Errorf("fork bomb action = %v, want Deny", d.Action)
	}
}

// TestPolicy_gitPushForceAsk 验证 git push --force 触发 Ask。
func TestPolicy_gitPushForceAsk(t *testing.T) {
	p := NewPolicy()
	args := json.RawMessage(`{"command":"git push --force origin main"}`)
	d := p.Check("bash", args)
	if d.Action != ActionAsk {
		t.Errorf("git push --force action = %v, want Ask", d.Action)
	}
}

// TestPolicy_laterUserRuleWins 验证后添加的用户规则优先级高于先添加的。
func TestPolicy_laterUserRuleWins(t *testing.T) {
	p := NewPolicy()
	// 先添加 Allow
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionAllow, Desc: "first allow"})
	// 后添加 Deny（应优先）
	p.AddUserRule(Rule{Tool: "bash", Args: []string{"ls"}, Action: ActionDeny, Desc: "second deny"})

	args := json.RawMessage(`{"command":"ls -la"}`)
	d := p.Check("bash", args)
	if d.Action != ActionDeny {
		t.Errorf("later user rule should win, action = %v, want Deny", d.Action)
	}
	if d.Matched != "second deny" {
		t.Errorf("matched = %q, want 'second deny'", d.Matched)
	}
}
