package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// launchUpdateInstaller 启动一个脱离当前进程的助手脚本：等本进程退出后安装
// 已下载的安装包，然后重新打开应用。当前进程随后应当退出。
func launchUpdateInstaller(archive string) error {
	if strings.TrimSpace(archive) == "" {
		return errors.New("安装包路径为空")
	}
	if _, err := os.Stat(archive); err != nil {
		return fmt.Errorf("安装包不存在：%w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return launchDarwinUpdate(archive)
	case "windows":
		return launchWindowsUpdate(archive)
	default:
		return errors.New("当前平台暂不支持自动安装，请到发布页面手动下载")
	}
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// startDetached 启动一个不会随本进程退出而被回收的助手进程。
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = detachedSysProcAttr()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// --- macOS ---------------------------------------------------------------

const darwinUpdateScript = `#!/bin/sh
set -u
PID={{PID}}
APP={{APP}}
WORK={{WORK}}
ARCHIVE={{ARCHIVE}}
MOUNT="$WORK/mnt"
EXTRACT="$WORK/extract"

while kill -0 "$PID" 2>/dev/null; do
  sleep 0.5
done

SRC=""
case "$ARCHIVE" in
  *.dmg)
    mkdir -p "$MOUNT"
    if hdiutil attach "$ARCHIVE" -nobrowse -quiet -mountpoint "$MOUNT"; then
      for candidate in "$MOUNT"/*.app; do
        if [ -d "$candidate" ]; then SRC="$candidate"; break; fi
      done
    fi
    ;;
  *)
    if ditto -x -k "$ARCHIVE" "$EXTRACT"; then
      for candidate in "$EXTRACT"/*.app; do
        if [ -d "$candidate" ]; then SRC="$candidate"; break; fi
      done
    fi
    ;;
esac

if [ -z "$SRC" ]; then
  hdiutil detach "$MOUNT" -quiet 2>/dev/null
  exit 1
fi

rm -rf "$APP.old"
if mv "$APP" "$APP.old" 2>/dev/null && ditto "$SRC" "$APP"; then
  rm -rf "$APP.old"
  xattr -dr com.apple.quarantine "$APP" 2>/dev/null
else
  rm -rf "$APP"
  mv "$APP.old" "$APP" 2>/dev/null
fi

hdiutil detach "$MOUNT" -quiet 2>/dev/null
rm -f "$ARCHIVE"
rm -rf "$EXTRACT"
rm -rf "$WORK"
open "$APP"
`

func launchDarwinUpdate(archive string) error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	appPath, err := macOSBundlePath(exe)
	if err != nil {
		return err
	}
	// 直接从 .dmg 挂载卷里运行时无法就地替换，提示用户先安装到本地磁盘。
	if strings.HasPrefix(appPath, "/Volumes/") {
		return errors.New("应用正从磁盘映像运行，请先把 Veda 拖到「应用程序」文件夹")
	}
	if err := ensureWritableDir(filepath.Dir(appPath)); err != nil {
		return errors.New("应用所在目录不可写，无法自动替换，请手动下载安装包")
	}
	work, err := os.MkdirTemp("", "veda-update-")
	if err != nil {
		return err
	}
	script := filepath.Join(work, "apply.sh")
	body := strings.NewReplacer(
		"{{PID}}", fmt.Sprintf("%d", os.Getpid()),
		"{{APP}}", shellQuote(appPath),
		"{{WORK}}", shellQuote(work),
		"{{ARCHIVE}}", shellQuote(archive),
	).Replace(darwinUpdateScript)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		return err
	}
	return startDetached("/bin/sh", script)
}

// macOSBundlePath 从 Contents/MacOS/Veda 反推出 .app 目录。
func macOSBundlePath(exe string) (string, error) {
	dir := filepath.Dir(exe)
	for {
		if strings.HasSuffix(filepath.Base(dir), ".app") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("当前不是从 .app 包中运行，无法自动安装")
		}
		dir = parent
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// --- Windows -------------------------------------------------------------

const windowsUpdateScript = `@echo off
setlocal
set "PID={{PID}}"
set "INSTALLER={{INSTALLER}}"
set "APPEXE={{APP}}"

:wait
tasklist /FI "PID eq %PID%" 2>NUL | find "%PID%" >NUL
if not errorlevel 1 (
  ping -n 2 127.0.0.1 >NUL
  goto wait
)

start "" /wait "%INSTALLER%" /S

for /f "tokens=2,*" %%A in ('reg query "HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\VedaVeda" /v DisplayIcon 2^>NUL ^| findstr /I "DisplayIcon"') do set "APPEXE=%%B"
if not exist "%APPEXE%" set "APPEXE={{APP}}"

start "" "%APPEXE%"
del "%~f0" >NUL 2>&1
`

func launchWindowsUpdate(archive string) error {
	exe, err := executablePath()
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "veda-update-")
	if err != nil {
		return err
	}
	script := filepath.Join(work, "apply.cmd")
	body := strings.NewReplacer(
		"{{PID}}", fmt.Sprintf("%d", os.Getpid()),
		"{{INSTALLER}}", archive,
		"{{APP}}", exe,
	).Replace(windowsUpdateScript)
	body = strings.ReplaceAll(body, "\n", "\r\n")
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		return err
	}
	return startDetached("cmd", "/C", script)
}

// ensureWritableDir 通过实际创建临时文件来判断目录是否可写。
func ensureWritableDir(dir string) error {
	file, err := os.CreateTemp(dir, ".veda-write-test-*")
	if err != nil {
		return err
	}
	name := file.Name()
	_ = file.Close()
	_ = os.Remove(name)
	return nil
}
