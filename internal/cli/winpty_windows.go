//go:build windows

package cli

import (
	"os"
	"strings"
)

// isMintty 检测当前是否运行在 Git Bash/mintty 终端下。
//
// mintty 与 chzyer/readline 不兼容的根因：
// mintty 是 Cygwin PTY，而 chzyer/readline 用 Windows Console API，
// 二者协议不兼容，readline 第一次 Readline() 立即返回 EOF。
//
// 检测逻辑：
// 1. Windows 原生 cmd/PowerShell 的 TERM 为空 → false
// 2. Git Bash(mintty) 的 TERM=xterm* → 候选
// 3. 同时检查 MSYSTEM（Git Bash 特有，如 MINGW64/MSYS）→ 双重确认
//
// 注意：检测到 mintty 后不自动 winpty 包装（曾尝试自动包装，
// 但 winpty 会吞掉子进程的错误输出，导致闪退时无法定位问题）。
// 改为仅打印提示，用户可手动运行 winpty trae interactive。
func isMintty() bool {
	// Windows 原生终端的 TERM 为空
	term := os.Getenv("TERM")
	if term == "" {
		return false
	}
	// mintty 的 TERM 通常是 xterm 或 xterm-256color
	if !strings.HasPrefix(term, "xterm") {
		return false
	}
	// Git Bash 会设置 MSYSTEM=MSYS 或 MINGW64/MINGW32
	// 双重确认避免误判其他 Cygwin 终端
	if os.Getenv("MSYSTEM") == "" {
		return false
	}
	return true
}
