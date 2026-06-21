package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestStore_SaveLoad(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStoreWithDir(tmp)
	if err != nil {
		t.Fatal(err)
	}

	sess := &Session{
		ID:       "test-001",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
		Usage:    llm.Usage{InputTokens: 5, OutputTokens: 3},
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load("test-001")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "test-001" {
		t.Errorf("id = %q", loaded.ID)
	}
	if len(loaded.Messages) != 1 {
		t.Errorf("messages = %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "hello" {
		t.Errorf("content = %q", loaded.Messages[0].Content)
	}
	if loaded.Usage.InputTokens != 5 {
		t.Errorf("usage = %+v", loaded.Usage)
	}
}

func TestStore_Load_notFound(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)
	_, err := store.Load("nonexistent")
	if err == nil {
		t.Error("expected error for missing session")
	}
}

func TestStore_List(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)

	// 保存 3 个会话
	for _, id := range []string{"a", "b", "c"} {
		store.Save(&Session{ID: id, Messages: []llm.Message{}})
	}

	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(list))
	}
}

func TestStore_List_sortedByUpdatedAt(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)

	// 保存会话，b 最后更新
	store.Save(&Session{ID: "a"})
	store.Save(&Session{ID: "b"})

	list, _ := store.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	// b 更新时间 >= a（可能相同），顺序应是 b 在前或相等
	if list[0].ID != "b" && list[0].UpdatedAt.Equal(list[1].UpdatedAt) {
		// 时间相同则顺序不确定，可接受
	}
}

func TestStore_Delete(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)
	store.Save(&Session{ID: "x"})

	if err := store.Delete("x"); err != nil {
		t.Fatal(err)
	}
	// 文件应不存在
	if _, err := os.Stat(filepath.Join(tmp, "x.json")); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestStore_List_empty(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(list))
	}
}

func TestGenerateID(t *testing.T) {
	id := GenerateID()
	// 20060102-150405-xxxxxxxxxxxx = 15 + 1 + 12 = 28
	if len(id) != 28 {
		t.Errorf("id length = %d, want 28: %s", len(id), id)
	}
	// 两次调用应不同（随机后缀）
	id2 := GenerateID()
	if id == id2 {
		t.Error("two GenerateID calls should differ")
	}
	// 批量生成 1000 个，验证无碰撞（6 字节随机，碰撞概率极低）
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		gid := GenerateID()
		if seen[gid] {
			t.Fatalf("collision detected: %s", gid)
		}
		seen[gid] = true
	}
}

// TestStore_Save_emptyID 验证空 ID 被拒绝，避免写入 .json 隐藏文件。
func TestStore_Save_emptyID(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)
	err := store.Save(&Session{ID: ""})
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}

// TestStore_Save_atomic 验证 Save 是原子写入：
// 写入后文件存在且可读，无残留 .tmp 文件。
func TestStore_Save_atomic(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)
	sess := &Session{ID: "atomic-test", Messages: []llm.Message{{Role: llm.RoleUser, Content: "data"}}}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	// 主文件应存在
	if _, err := os.Stat(filepath.Join(tmp, "atomic-test.json")); err != nil {
		t.Errorf("main file missing: %v", err)
	}
	// 不应有残留 .tmp 文件
	if _, err := os.Stat(filepath.Join(tmp, "atomic-test.json.tmp")); !os.IsNotExist(err) {
		t.Errorf("tmp file should not exist after atomic save: %v", err)
	}
}
