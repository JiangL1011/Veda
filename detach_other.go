//go:build !windows

package main

import "syscall"

// detachedSysProcAttr 在类 Unix 系统上不需要额外属性：父进程退出后，
// 助手进程会被 init（或 launchd）接管并继续运行。
func detachedSysProcAttr() *syscall.SysProcAttr {
	return nil
}
