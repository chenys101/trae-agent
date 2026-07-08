//go:build windows

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// winptyEnvMark 标记当前进程已通过 winpty 包装，避免无限递归。
const winptyEnvMark = "TRAE_WINPTY"

// shouldUseWinpty 检测当前是否运行在 mintty 下且 winpty 可用。
//
// 检测逻辑：
// 1. 已在 winpty 下（TRAE_WINPTY=1）→ 返回 false，避免递归
// 2. Windows 原生 cmd/PowerShell 的 TERM 为空 → 返回 false
// 3. mintty 下 TERM=xterm 或 xterm-256color → 候选
// 4. winpty 命令不存在 → 返回 false（无可行包装方案）
//
// mintty 与 chzyer/readline 不兼容的根因：
// mintty 是 Cygwin PTY，而 chzyer/readline 用 Windows Console API，
// 二者协议不兼容。winpty 提供 PTY→Console 桥接，让 readline 误以为
// 是真正的 Windows Console。
func shouldUseWinpty() bool {
	// 已在 winpty 下，正常执行
	if os.Getenv(winptyEnvMark) != "" {
		return false
	}
	// Windows 原生终端的 TERM 为空
	// mintty 的 TERM 通常是 xterm 或 xterm-256color
	term := os.Getenv("TERM")
	if term == "" {
		return false
	}
	// 严格匹配 xterm 前缀，避免误判其他场景
	if !strings.HasPrefix(term, "xterm") {
		return false
	}
	// 检测 winpty 是否可用
	if _, err := exec.LookPath("winpty"); err != nil {
		return false
	}
	return true
}

// execViaWinpty 用 winpty 重新启动当前命令，转发所有参数。
// 等待子进程退出，返回退出码。失败时返回错误。
//
// 调用方应在 shouldUseWinpty 返回 true 时调用此函数，
// 然后立即 os.Exit(code)，不再执行后续逻辑。
func execViaWinpty() (int, error) {
	// winpty -- <可执行文件路径> <参数...>
	// winpty 会启动一个隐藏的 Windows Console，将 PTY I/O 桥接到它
	cmdArgs := append([]string{"--"}, os.Args...)
	cmd := exec.Command("winpty", cmdArgs...)
	// 标记环境变量，避免子进程再次尝试 winpty 包装
	cmd.Env = append(os.Environ(), winptyEnvMark+"=1")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// 子进程非 0 退出码：传递退出码
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		// winpty 自身错误（如找不到、IO 错误）
		return 1, fmt.Errorf("winpty wrap failed: %w", err)
	}
	return 0, nil
}
