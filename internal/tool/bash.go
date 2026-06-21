package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// maxBashOutput bash 输出上限（1MB），超过截断并附加提示。
const maxBashOutput = 1 << 20

// maxBashTimeoutMs bash 超时上限（600s），超过拒绝以避免长时间挂起。
const maxBashTimeoutMs = 600000

type Bash struct {
	defaultTimeout time.Duration
}

func NewBash(defaultTimeout time.Duration) *Bash {
	if defaultTimeout <= 0 {
		defaultTimeout = 120 * time.Second
	}
	return &Bash{defaultTimeout: defaultTimeout}
}

func (Bash) Name() string        { return "bash" }
func (Bash) Description() string { return "Execute a bash command. Returns combined stdout+stderr." }

func (Bash) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {"type": "string", "description": "bash command to execute"},
    "workdir": {"type": "string", "description": "working directory, default cwd"},
    "timeout_ms": {"type": "integer", "description": "timeout in milliseconds"}
  },
  "required": ["command"]
}`)
}

type bashArgs struct {
	Command   string `json:"command"`
	Workdir   string `json:"workdir"`
	TimeoutMs int    `json:"timeout_ms"`
}

func (b *Bash) Run(ctx context.Context, args json.RawMessage) Result {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.Command == "" {
		return ErrorResult("command is required")
	}

	if a.TimeoutMs > maxBashTimeoutMs {
		return ErrorResult("timeout_ms too large (max %dms)", maxBashTimeoutMs)
	}
	timeout := b.defaultTimeout
	if a.TimeoutMs > 0 {
		timeout = time.Duration(a.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", a.Command)
	if a.Workdir != "" {
		cmd.Dir = a.Workdir
	}
	// 设置独立进程组，cancel 时 kill 整个进程组，
	// 避免 bash 派生的子进程（如 `sleep 100 &`）在父进程退出后仍存活
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// context 取消时杀整个进程组（负 PID 表示进程组）
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return os.ErrProcessDone
	}

	// 用 LimitWriter 限制输出大小，避免 `yes` / `cat /dev/zero` 等命令导致 OOM
	var buf bytes.Buffer
	limited := &limitedWriter{w: &buf, max: maxBashOutput}
	cmd.Stdout = limited
	cmd.Stderr = limited

	// 构造白名单环境，过滤掉含 KEY/SECRET/TOKEN/PASSWORD 的敏感变量，
	// 避免通过环境变量向子进程泄漏密钥。
	cmd.Env = filterEnv(os.Environ())

	err := cmd.Run()
	output := buf.String()
	if limited.truncated && output != "" {
		output += "\n...[output truncated at 1MB]"
	}
	// 先检查进程是否成功退出，再判断 ctx.Err()，避免竞态：
	// ctx 可能已超时但命令恰好在 cancel 触发前正常退出。
	if cmd.ProcessState != nil && cmd.ProcessState.Success() {
		return Result{Content: output}
	}
	if err != nil {
		if ctx.Err() != nil {
			return ErrorResult("cancelled: %v\n%s", ctx.Err(), output)
		}
		return ErrorResult("exit: %v\n%s", err, output)
	}
	return Result{Content: output}
}

// limitedWriter 包装一个 Writer，写入超过 max 字节后丢弃后续写入并标记 truncated。
type limitedWriter struct {
	w         *bytes.Buffer
	max       int
	written   int
	truncated bool
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.truncated {
		return len(p), nil
	}
	remaining := l.max - l.written
	if remaining <= 0 {
		l.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		n, _ := l.w.Write(p[:remaining])
		l.written += n
		l.truncated = true
		return len(p), nil
	}
	n, err := l.w.Write(p)
	l.written += n
	return n, err
}

// 确保 limitedWriter 实现 io.Writer 接口（编译期检查）
var _ interface {
	Write([]byte) (int, error)
} = (*limitedWriter)(nil)

// filterEnv 过滤环境变量，剔除名称中含 KEY/SECRET/TOKEN/PASSWORD 的敏感变量，
// 避免通过子进程环境泄漏密钥。
func filterEnv(env []string) []string {
	var out []string
	for _, e := range env {
		idx := strings.Index(e, "=")
		if idx < 0 {
			continue
		}
		key := strings.ToUpper(e[:idx])
		if strings.Contains(key, "KEY") || strings.Contains(key, "SECRET") ||
			strings.Contains(key, "TOKEN") || strings.Contains(key, "PASSWORD") {
			continue
		}
		out = append(out, e)
	}
	return out
}
