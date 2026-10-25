//go:build !windows

package platform

import "syscall"

// DetachedSysProcAttr 在类 Unix 系统上不需要额外属性：父进程退出后，
// 助手进程会被 init（或 launchd）接管并继续运行。
func DetachedSysProcAttr() *syscall.SysProcAttr {
	return nil
}
