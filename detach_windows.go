//go:build windows

package main

import "syscall"

// detachedSysProcAttr 让安装更新用的 cmd 进程在无窗口模式下运行，
// 避免 GUI 程序拉起控制台时闪出一个黑框。
func detachedSysProcAttr() *syscall.SysProcAttr {
	const createNoWindow = 0x08000000
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
