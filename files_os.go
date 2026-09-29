package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func revealInFileManager(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", abs).Run()
	case "windows":
		// explorer.exe 即便执行成功也会返回非零状态码。
		_ = exec.Command("explorer", "/select,", abs).Run()
		return nil
	default:
		return revealLinux(abs)
	}
}

func revealLinux(abs string) error {
	uri := fileURI(abs)
	cmd := exec.Command(
		"dbus-send",
		"--session",
		"--dest=org.freedesktop.FileManager1",
		"--type=method_call",
		"/org/freedesktop/FileManager1",
		"org.freedesktop.FileManager1.ShowItems",
		"array:string:"+uri,
		"string:",
	)
	if err := cmd.Run(); err == nil {
		return nil
	}
	info, err := os.Stat(abs)
	dir := abs
	if err == nil && !info.IsDir() {
		dir = filepath.Dir(abs)
	}
	return exec.Command("xdg-open", dir).Start()
}

func moveToTrash(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return trashDarwin(abs)
	case "windows":
		return trashWindows(abs, info.IsDir())
	default:
		return trashLinux(abs)
	}
}

// trashDarwin 通过 Finder 执行真正可“放回原处”的删除；当 Finder 自动化不可用时，
// 退而直接移动到 ~/.Trash。
func trashDarwin(abs string) error {
	script := fmt.Sprintf(`tell application "Finder" to delete POSIX file "%s"`, appleScriptEscape(abs))
	if err := exec.Command("osascript", "-e", script).Run(); err == nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	trash := filepath.Join(home, ".Trash")
	if err := os.MkdirAll(trash, 0o700); err != nil {
		return err
	}
	dest, _, err := uniqueChildPath(trash, filepath.Base(abs))
	if err != nil {
		return err
	}
	if err := os.Rename(abs, dest); err != nil {
		return errors.New("无法移入废纸篓")
	}
	return nil
}

func trashWindows(abs string, isDir bool) error {
	target := "DeleteFile"
	if isDir {
		target = "DeleteDirectory"
	}
	script := fmt.Sprintf(
		"Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.FileIO.FileSystem]::%s(%s,'OnlyErrorDialogs','SendToRecycleBin')",
		target,
		powershellQuote(abs),
	)
	return exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Run()
}

func trashLinux(abs string) error {
	if err := exec.Command("gio", "trash", "--", abs).Run(); err == nil {
		return nil
	}
	if err := exec.Command("gvfs-trash", abs).Run(); err == nil {
		return nil
	}
	return errors.New("系统未提供回收站服务")
}

func copyFileToClipboard(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`set the clipboard to POSIX file "%s"`, appleScriptEscape(abs))
		return exec.Command("osascript", "-e", script).Run()
	case "windows":
		return exec.Command(
			"powershell",
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"Set-Clipboard -LiteralPath "+powershellQuote(abs),
		).Run()
	default:
		return copyFileLinux(abs)
	}
}

func copyFileLinux(abs string) error {
	uri := fileURI(abs) + "\n"
	if err := pipeTo("wl-copy", []string{"--type", "text/uri-list"}, uri); err == nil {
		return nil
	}
	if err := pipeTo("xclip", []string{"-selection", "clipboard", "-t", "text/uri-list"}, uri); err == nil {
		return nil
	}
	return errors.New("无法写入剪贴板")
}

func fileURI(abs string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

func appleScriptEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, `"`, `\"`)
}

func powershellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func pipeTo(name string, args []string, input string) error {
	cmd := exec.Command(name, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}
	_, _ = io.WriteString(stdin, input)
	_ = stdin.Close()
	return cmd.Wait()
}
