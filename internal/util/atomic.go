// Package util 提供跨平台共享工具函数。
package util

import (
	"os"
	"path/filepath"
)

// AtomicWrite 原子写入文件：先写临时文件再 rename，避免写入中途崩溃导致文件损坏。
// Windows 上 os.Rename 可能因目标文件被占用（如 antivirus 扫描）而失败，
// 此时回退到直接覆盖写入（非原子但保证可用）。
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	// 临时文件放在同目录，确保同卷（跨卷 rename 会失败）
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		// 临时文件创建失败，回退到直接写入
		return os.WriteFile(path, data, perm)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // cleanup 兜底

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		return err
	}

	// 尝试原子 rename，失败则回退到直接写入
	if err := os.Rename(tmpPath, path); err != nil {
		return os.WriteFile(path, data, perm)
	}
	return nil
}
