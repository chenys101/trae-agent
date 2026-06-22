package permission

import (
	"context"
	"testing"
)

// TestAutoAsker_defaultDeny 验证 AutoAsker 默认返回 Deny。
func TestAutoAsker_defaultDeny(t *testing.T) {
	a := &AutoAsker{Default: ActionDeny}
	got := a.Ask(context.Background(), "bash", "rm -rf /tmp", "dangerous")
	if got != ActionDeny {
		t.Errorf("AutoAsker(Deny).Ask = %v, want Deny", got)
	}
}

// TestAutoAsker_defaultAllow 验证 AutoAsker 可配置为 Allow。
func TestAutoAsker_defaultAllow(t *testing.T) {
	a := &AutoAsker{Default: ActionAllow}
	got := a.Ask(context.Background(), "bash", "rm -rf /tmp", "dangerous")
	if got != ActionAllow {
		t.Errorf("AutoAsker(Allow).Ask = %v, want Allow", got)
	}
}

// TestAutoAsker_ignoresArgs 验证 AutoAsker 忽略参数，始终返回 Default。
func TestAutoAsker_ignoresArgs(t *testing.T) {
	a := &AutoAsker{Default: ActionDeny}
	// 不同参数都应返回 Default
	cases := []struct{ tool, args, reason string }{
		{"bash", "rm -rf /", "dangerous"},
		{"read", "/etc/passwd", "sensitive"},
		{"write", "/tmp/file", "harmless"},
	}
	for _, c := range cases {
		got := a.Ask(context.Background(), c.tool, c.args, c.reason)
		if got != ActionDeny {
			t.Errorf("AutoAsker.Ask(%+v) = %v, want Deny", c, got)
		}
	}
}

// TestCallbackAsker_invokesFunc 验证 CallbackAsker 调用传入的函数。
func TestCallbackAsker_invokesFunc(t *testing.T) {
	called := false
	c := &CallbackAsker{
		Func: func(ctx context.Context, tool, args, reason string) Action {
			called = true
			if tool != "bash" {
				t.Errorf("tool = %q, want bash", tool)
			}
			if args != "rm -rf /tmp" {
				t.Errorf("args = %q, want 'rm -rf /tmp'", args)
			}
			if reason != "dangerous" {
				t.Errorf("reason = %q, want 'dangerous'", reason)
			}
			return ActionAllow
		},
	}
	got := c.Ask(context.Background(), "bash", "rm -rf /tmp", "dangerous")
	if !called {
		t.Error("callback was not invoked")
	}
	if got != ActionAllow {
		t.Errorf("CallbackAsker.Ask = %v, want Allow", got)
	}
}

// TestCallbackAsker_nilFuncDenies 验证 CallbackAsker 的 Func 为 nil 时返回 Deny（安全默认）。
func TestCallbackAsker_nilFuncDenies(t *testing.T) {
	c := &CallbackAsker{} // Func 为 nil
	got := c.Ask(context.Background(), "bash", "rm -rf /tmp", "dangerous")
	if got != ActionDeny {
		t.Errorf("CallbackAsker with nil Func = %v, want Deny", got)
	}
}

// TestCallbackAsker_returnsActionFromFunc 验证 CallbackAsker 返回函数返回的 Action。
func TestCallbackAsker_returnsActionFromFunc(t *testing.T) {
	cases := []struct {
		name   string
		action Action
	}{
		{"allow", ActionAllow},
		{"deny", ActionDeny},
		{"ask", ActionAsk},
		{"allow_always", ActionAllowAlways},
		{"deny_always", ActionDenyAlways},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cb := &CallbackAsker{
				Func: func(ctx context.Context, tool, args, reason string) Action {
					return c.action
				},
			}
			got := cb.Ask(context.Background(), "bash", "cmd", "reason")
			if got != c.action {
				t.Errorf("got %v, want %v", got, c.action)
			}
		})
	}
}

// TestAutoAsker_allowAlways 验证 AutoAsker 可配置为 AllowAlways。
func TestAutoAsker_allowAlways(t *testing.T) {
	a := &AutoAsker{Default: ActionAllowAlways}
	got := a.Ask(context.Background(), "bash", "git push", "needs confirm")
	if got != ActionAllowAlways {
		t.Errorf("AutoAsker(AllowAlways).Ask = %v, want AllowAlways", got)
	}
}
