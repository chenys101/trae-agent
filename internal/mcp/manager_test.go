package mcp

import (
	"context"
	"testing"
)

// TestManager_StartAll_NoServers 测试启动空 server 列表。
func TestManager_StartAll_NoServers(t *testing.T) {
	m := NewManager()
	defer m.Close()

	errs := m.StartAll(context.Background(), nil)
	if len(errs) != 0 {
		t.Errorf("StartAll with nil servers returned %d errors, want 0", len(errs))
	}

	if got := len(m.Servers()); got != 0 {
		t.Errorf("len(Servers) = %d, want 0", got)
	}
	if got := len(m.Tools()); got != 0 {
		t.Errorf("len(Tools) = %d, want 0", got)
	}
}

// TestManager_ServersReturnsStatus 测试 Servers 返回状态。
func TestManager_ServersReturnsStatus(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// 无 server 时返回空列表
	statuses := m.Servers()
	if len(statuses) != 0 {
		t.Errorf("len(Servers) = %d, want 0", len(statuses))
	}
}

// TestManager_CloseIdempotent 测试 Close 幂等。
func TestManager_CloseIdempotent(t *testing.T) {
	m := NewManager()
	if err := m.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("third Close: %v", err)
	}
}

// TestManager_ToolsEmptyAfterClose 测试 Close 后 Tools 为空。
func TestManager_ToolsEmptyAfterClose(t *testing.T) {
	m := NewManager()
	m.Close()
	if got := len(m.Tools()); got != 0 {
		t.Errorf("len(Tools) after Close = %d, want 0", got)
	}
}
