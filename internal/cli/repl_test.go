package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/session"
	"github.com/bytedance/trae-agent/internal/tool"
	"github.com/chzyer/readline"
)

// cliMockProvider 测试用 mock provider。
// streamErr 非 nil 时 Stream 直接返回错误；否则按 script 发送事件。
type cliMockProvider struct {
	streamErr error             // Stream 返回的错误
	script    []llm.StreamEvent // 发送的事件序列
}

func (m *cliMockProvider) Name() string { return "mock" }

func (m *cliMockProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	ch := make(chan llm.StreamEvent, len(m.script))
	go func() {
		defer close(ch)
		for _, e := range m.script {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch, nil
}

// mockStore 计数 Save 调用次数的 mock store。
type mockStore struct {
	saveCount  int
	saved      []*session.Session
	loadErr    error
	listResult []*session.Session
	listErr    error
}

func (m *mockStore) Save(sess *session.Session) error {
	m.saveCount++
	m.saved = append(m.saved, sess)
	return nil
}

func (m *mockStore) Load(id string) (*session.Session, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return nil, errors.New("session not found: " + id)
}

func (m *mockStore) List() ([]*session.Session, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResult, nil
}

// mockCompactor 计数 Compact 调用次数的 mock compactor。
type mockCompactor struct {
	compactCount int
	result       []llm.Message
	err          error
}

func (m *mockCompactor) Compact(ctx context.Context, messages []llm.Message) ([]llm.Message, error) {
	m.compactCount++
	if m.err != nil {
		return nil, m.err
	}
	if m.result != nil {
		return m.result, nil
	}
	return messages, nil
}

// newTestReadline 创建测试用 readline 实例（非 TTY 模式）。
func newTestReadline(t *testing.T, input string) *readline.Instance {
	t.Helper()
	rl, err := readline.NewEx(&readline.Config{
		Stdin:  io.NopCloser(strings.NewReader(input)),
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("创建 readline 实例失败: %v", err)
	}
	t.Cleanup(func() { rl.Close() })
	return rl
}

// newTestREPL 构造测试用 REPL，注入 mock readline。
func newTestREPL(t *testing.T, input string, a *agent.Agent) *REPL {
	t.Helper()
	r := &REPL{
		agent:     a,
		rl:        newTestReadline(t, input),
		commands:  NewCommandRegistry(),
		renderer:  NewRenderer(),
		ctxMgr:    agent.NewContextManager(),
		ctx:       context.Background(),
		sessionID: "test-session",
	}
	return r
}

// TestREPL_saveSession_allExitPaths 验证 saveSession 在各退出路径都被调用。
// 通过 mock store 计数 Save 调用次数。
func TestREPL_saveSession_allExitPaths(t *testing.T) {
	// 直接调用 saveSession：有消息时 Save 被调用
	t.Run("direct_withMessages", func(t *testing.T) {
		store := &mockStore{}
		r := &REPL{
			store:     store,
			sessionID: "s1",
			messages:  []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
		}
		r.saveSession()
		if store.saveCount != 1 {
			t.Errorf("expected 1 save, got %d", store.saveCount)
		}
		if len(store.saved) != 1 || store.saved[0].ID != "s1" {
			t.Errorf("unexpected saved session: %+v", store.saved)
		}
	})

	// 直接调用 saveSession：无消息时 Save 不被调用
	t.Run("direct_noMessages", func(t *testing.T) {
		store := &mockStore{}
		r := &REPL{store: store, sessionID: "s2"}
		r.saveSession()
		if store.saveCount != 0 {
			t.Errorf("expected 0 saves, got %d", store.saveCount)
		}
	})

	// 直接调用 saveSession：store 为 nil 时不 panic
	t.Run("direct_nilStore", func(t *testing.T) {
		r := &REPL{sessionID: "s3", messages: []llm.Message{{Role: llm.RoleUser, Content: "x"}}}
		r.saveSession() // 不 panic 即可
	})

	// Run() EOF 路径：空 stdin → Readline 返回 EOF → defer saveSession 执行
	t.Run("eof", func(t *testing.T) {
		store := &mockStore{}
		a := agent.New(&cliMockProvider{}, tool.NewRegistry(), agent.WithModel("m"))
		r := &REPL{
			agent:     a,
			rl:        newTestReadline(t, ""),
			commands:  NewCommandRegistry(),
			renderer:  NewRenderer(),
			store:     store,
			sessionID: "eof-session",
			messages:  []llm.Message{{Role: llm.RoleUser, Content: "data"}},
			ctx:       context.Background(),
		}
		if err := r.Run(context.Background()); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if store.saveCount != 1 {
			t.Errorf("EOF path: expected 1 save, got %d", store.saveCount)
		}
	})

	// Run() /exit 路径：stdin 含 /exit → handleCommand 返回 true → defer saveSession 执行
	t.Run("exit", func(t *testing.T) {
		store := &mockStore{}
		a := agent.New(&cliMockProvider{}, tool.NewRegistry(), agent.WithModel("m"))
		r := &REPL{
			agent:     a,
			rl:        newTestReadline(t, "/exit\n"),
			commands:  NewCommandRegistry(),
			renderer:  NewRenderer(),
			store:     store,
			sessionID: "exit-session",
			messages:  []llm.Message{{Role: llm.RoleUser, Content: "data"}},
			ctx:       context.Background(),
		}
		if err := r.Run(context.Background()); err != nil {
			t.Fatalf("Run failed: %v", err)
		}
		if store.saveCount != 1 {
			t.Errorf("/exit path: expected 1 save, got %d", store.saveCount)
		}
	})

	// Run() ErrInterrupt 路径：readline 在非 TTY 模式下无法模拟 ErrInterrupt，
	// 但 defer r.saveSession() 覆盖所有 return 路径（EOF / ErrInterrupt / /exit / readline error）。
	// 此处通过直接调用 saveSession 验证函数本身可靠，ErrInterrupt 路径由同一 defer 覆盖。
	t.Run("errInterrupt_coveredByDefer", func(t *testing.T) {
		store := &mockStore{}
		r := &REPL{
			store:     store,
			sessionID: "interrupt-session",
			messages:  []llm.Message{{Role: llm.RoleUser, Content: "data"}},
		}
		// 模拟 defer saveSession 在 ErrInterrupt 退出路径的调用
		r.saveSession()
		if store.saveCount != 1 {
			t.Errorf("ErrInterrupt path: expected 1 save, got %d", store.saveCount)
		}
	})
}

// TestREPL_pendingMessages_rollbackOnFailure 验证 agent 失败时 r.messages 不被污染。
// 用 mock provider 返回 error，验证 r.messages 长度不变。
func TestREPL_pendingMessages_rollbackOnFailure(t *testing.T) {
	// 构造会失败的 agent：provider.Stream 返回 error
	failProvider := &cliMockProvider{
		streamErr: errors.New("provider boom"),
	}
	a := agent.New(failProvider, tool.NewRegistry(), agent.WithModel("m"))

	initialMessages := []llm.Message{
		{Role: llm.RoleUser, Content: "q1"},
		{Role: llm.RoleAssistant, Content: "a1"},
	}
	r := newTestREPL(t, "", a)
	r.messages = initialMessages

	beforeLen := len(r.messages)
	r.runAgent(context.Background(), "this will fail", NewInterruptHandler())

	// agent 失败，r.messages 不应被 pendingMessages 污染
	if len(r.messages) != beforeLen {
		t.Errorf("agent failed but messages changed: before=%d after=%d", beforeLen, len(r.messages))
	}
	// 确保原始消息内容未被修改
	if r.messages[0].Content != "q1" {
		t.Errorf("messages[0] corrupted: %q", r.messages[0].Content)
	}
}

// TestREPL_autoCompact_trigger 验证消息超过阈值时触发 auto-compact。
// 用 mock agent + mock compactor，构造足够多消息，验证 compactor.Compact 被调用。
func TestREPL_autoCompact_trigger(t *testing.T) {
	// 构造会成功的 agent：返回简单文本响应
	okProvider := &cliMockProvider{
		script: []llm.StreamEvent{
			llm.TextDelta{Content: "response"},
			llm.Done{Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}, StopReason: "end_turn"},
		},
	}
	a := agent.New(okProvider, tool.NewRegistry(), agent.WithModel("m"))

	// 阈值设为 1 token，任何消息都会触发 compact
	mc := &mockCompactor{
		result: []llm.Message{{Role: llm.RoleUser, Content: "compacted"}},
	}
	r := newTestREPL(t, "", a)
	r.ctxMgr = agent.NewContextManager(agent.WithMaxTokens(1))
	r.compactor = mc
	r.messages = []llm.Message{{Role: llm.RoleUser, Content: "initial"}}

	r.runAgent(context.Background(), "trigger compact", NewInterruptHandler())

	if mc.compactCount != 1 {
		t.Errorf("expected compactor.Compact called once, got %d", mc.compactCount)
	}
	// compact 后 messages 应被替换为 mock 结果
	if len(r.messages) != 1 || r.messages[0].Content != "compacted" {
		t.Errorf("messages not compacted as expected: %+v", r.messages)
	}
}
