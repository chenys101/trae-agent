package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/bytedance/trae-agent/internal/consts"
)

type Bash struct {
	defaultTimeout time.Duration
}

func NewBash(defaultTimeout time.Duration) *Bash {
	if defaultTimeout <= 0 {
		// 未传超时时回退到 consts 中定义的默认值，避免魔法数字散落
		defaultTimeout = consts.DefaultBashTimeout
	}
	return &Bash{defaultTimeout: defaultTimeout}
}

// shellName 返回当前平台的默认 shell 名。
// Unix 用 sh，Windows 用 cmd。保持工具名为 "bash" 以兼容已有权限规则。
func shellName() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "bash"
}

// shellFlag 返回执行单条命令的 flag（-c 或 /c）。
func shellFlag() string {
	if runtime.GOOS == "windows" {
		return "/c"
	}
	return "-c"
}

func (Bash) Name() string        { return "bash" }
func (Bash) Description() string { return "Execute a shell command. Returns combined stdout+stderr." }

func (Bash) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {"type": "string", "description": "shell command to execute"},
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

	if a.TimeoutMs > consts.MaxBashTimeoutMs {
		return ErrorResult("timeout_ms too large (max %dms)", consts.MaxBashTimeoutMs)
	}
	timeout := b.defaultTimeout
	if a.TimeoutMs > 0 {
		timeout = time.Duration(a.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 按平台选择 shell：Unix 用 bash -c，Windows 用 cmd /c
	cmd := exec.CommandContext(ctx, shellName(), shellFlag(), a.Command)
	if a.Workdir != "" {
		cmd.Dir = a.Workdir
	}
	// 设置进程组（Unix）或 Job Object（Windows），cancel 时杀整个进程组，
	// 避免 shell 派生的子进程在父进程退出后仍存活。
	// 具体实现见 exec_unix.go / exec_windows.go
	setProcessGroup(cmd)
	// context 取消时杀整个进程组
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			killProcessGroup(cmd.Process.Pid)
		}
		return os.ErrProcessDone
	}

	// 用 LimitWriter 限制输出大小，避免无限输出命令导致 OOM
	var buf bytes.Buffer
	limited := &limitedWriter{w: &buf, max: consts.MaxBashOutput}
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
		// idx <= 0 跳过：无 = 的行，以及空 key 的行
		// Windows 有 =C: 这类隐式驱动器变量，key 为空，需跳过
		if idx <= 0 {
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
