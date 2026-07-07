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

	"github.com/bytedance/trae-agent/internal/errors"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/paths"
	"github.com/bytedance/trae-agent/internal/util"
)

// CurrentVersion 当前会话 schema 版本。新增字段或改变结构时递增。
const CurrentVersion = 1

// Session 一次对话会话。
type Session struct {
	ID        string        `json:"id"`
	Version   int           `json:"version"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Messages  []llm.Message `json:"messages"`
	Usage     llm.Usage     `json:"usage"`
}

// Store 会话存储，持久化到用户主目录下 .trae/sessions/。
type Store struct {
	dir string
}

func NewStore() (*Store, error) {
	// 会话目录位于 ~/.trae/sessions/，由 paths 包统一创建（含 .trae 父目录）
	// 文件内容可能含敏感信息（prompt、粘贴的代码/密钥），保存时权限收紧为 0600（见 Save）
	dir, err := paths.UnderTrae("sessions")
	if err != nil {
		return nil, err
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
	// 旧会话（Version==0）保存时升级到当前版本；新会话也据此标记版本。
	if sess.Version == 0 {
		sess.Version = CurrentVersion
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return errors.Wrap(err, errors.CodeSessionSave, "marshal session")
	}
	path := filepath.Join(s.dir, sess.ID+".json")
	// 原子写入：先写临时文件再 rename，Windows 上 rename 失败时回退到直接写入
	if err := util.AtomicWrite(path, data, 0o600); err != nil {
		return errors.Wrap(err, errors.CodeSessionSave, "write session")
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
		// 文件不存在等读取错误保持原样返回，保留 os.IsNotExist 判断能力
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, errors.Wrap(err, errors.CodeSessionLoad, "unmarshal session")
	}
	return &sess, nil
}

// List 列出所有会话，按更新时间降序。
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeSessionLoad, "list sessions")
	}
	var sessions []*Session
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		id := name[:len(name)-len(".json")]
		if id == "" {
			continue // 跳过 .json（空 id）这类异常文件名
		}
		sess, err := s.Load(id)
		if err != nil {
			continue // 跳过损坏的会话文件
		}
		sessions = append(sessions, sess)
	}
	// 稳定排序：主键 UpdatedAt 降序，次级键 CreatedAt 降序，避免同 UpdatedAt 时顺序抖动。
	sort.SliceStable(sessions, func(i, j int) bool {
		if !sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
		}
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
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
