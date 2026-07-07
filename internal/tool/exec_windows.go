//go:build windows

package tool

import (
	"os/exec"
)

// setProcessGroup 在 Windows 上为空操作。
// Windows 不支持 Unix 进程组语义，改用 cmd.Process.Kill() 杀主进程即可。
// 子进程可能残留，但 cmd /c 通常不会派生长期运行的子进程。
func setProcessGroup(cmd *exec.Cmd) {
	// Windows 上不设置 SysProcAttr，使用默认行为
}

// killProcessGroup 在 Windows 上杀单个进程。
// Windows 不支持进程组概念，直接 Kill 主进程。
func killProcessGroup(pid int) {
	// Windows 上通过 cmd.Process.Kill() 由 cmd.Cancel 间接调用
	// 此函数在 Windows 上为 no-op，实际 kill 由 exec.Cmd 内部处理
}
