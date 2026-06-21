package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bytedance/trae-agent/internal/llm"
)

// Session 一次对话会话。
type Session struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Messages  []llm.Message `json:"messages"`
	Usage     llm.Usage     `json:"usage"`
}

// Store 会话存储，持久化到 ~/.trae/sessions/。
type Store struct {
	dir string
}

func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".trae", "sessions")
	// 会话内容可能含敏感信息（prompt、粘贴的代码/密钥），权限收紧为 0700
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// NewStoreWithDir 用指定目录构造（测试用）。
func NewStoreWithDir(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Save 保存会话。原子写入：先写临时文件再 rename，避免并发或中断损坏文件。
func (s *Store) Save(sess *Session) error {
	if sess.ID == "" {
		return fmt.Errorf("session ID is required")
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now()
	}
	sess.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, sess.ID+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp file: %w", err)
	}
	return nil
}

// validateID 校验会话 ID 只含安全字符，防止路径穿越。
func validateID(id string) error {
	if id == "" {
		return fmt.Errorf("session ID is required")
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("invalid session ID: %s (only [a-zA-Z0-9_-] allowed)", id)
		}
	}
	return nil
}

// Load 加载会话。
func (s *Store) Load(id string) (*Session, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// List 列出所有会话，按更新时间降序。
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		sess, err := s.Load(id)
		if err != nil {
			continue // 跳过损坏的会话文件
		}
		sessions = append(sessions, sess)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// Delete 删除会话。
func (s *Store) Delete(id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	path := filepath.Join(s.dir, id+".json")
	return os.Remove(path)
}

// GenerateID 生成会话 ID（时间戳 + 随机后缀，避免同秒冲突）。
// 随机后缀 6 字节（48 bit），同秒内生日攻击碰撞阈值约 16M 次生成。
func GenerateID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败极少见，但若发生不能让 b 全零导致 ID 碰撞。
		// 回退到时间戳的纳秒部分作为弱随机源。
		for i := range b {
			b[i] = byte(time.Now().UnixNano() >> (i * 8))
		}
	}
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}
