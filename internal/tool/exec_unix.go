//go:build !windows

package tool

import (
	"os/exec"
	"syscall"
)

// setProcessGroup 设置独立进程组（Unix 专属）。
// cancel 时 kill 整个进程组，避免 shell 派生的子进程在父进程退出后仍存活。
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup 杀整个进程组（负 PID 表示进程组）。
func killProcessGroup(pid int) {
	// 负 PID 表示杀整个进程组
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
