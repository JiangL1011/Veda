package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeFileAt 写入文件并把修改时间改成给定时刻，用来构造「旧安装包」。
func writeFileAt(t *testing.T, path string, data []byte, mod time.Time) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

// namesIn 返回目录里的条目名，用于断言清理结果。
func namesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

// TestCleanupStaleDownloadsKeepsNewestInstaller 验证启动清理只留下最近下载的那个安装包：
// 用户下载完还没点安装就退出的话，下次检查可以直接复用它，不必重新下载。
func TestCleanupStaleDownloadsKeepsNewestInstaller(t *testing.T) {
	dir := t.TempDir()
	updates := filepath.Join(dir, "updates")
	mustMkdirAll(t, updates)

	now := time.Now()
	writeFileAt(t, filepath.Join(updates, "Veda-0.1.0-macos-arm64.dmg"), []byte("old"), now.Add(-2*time.Hour))
	writeFileAt(t, filepath.Join(updates, "Veda-0.2.0-macos-arm64.dmg"), []byte("new"), now.Add(-time.Minute))
	writeFileAt(t, filepath.Join(updates, "Veda-0.3.0-macos-arm64.dmg"+downloadPartSuffix), []byte("half"), now)

	u := newUpdater()
	u.dataDir = func() (string, error) { return dir, nil }
	u.CleanupStaleDownloads()

	got := namesIn(t, updates)
	if len(got) != 1 || got[0] != "Veda-0.2.0-macos-arm64.dmg" {
		t.Fatalf("清理后剩余 %v，want [Veda-0.2.0-macos-arm64.dmg]", got)
	}
}

// TestUpdaterRemovesOlderDownloads 验证下载新版本时会把旧版本的安装包和下载残留一起清掉。
func TestUpdaterRemovesOlderDownloads(t *testing.T) {
	payload := []byte("installer-bytes")
	server, u := testReleaseServer(t, "v0.2.0", "Veda-0.2.0-macos-arm64.dmg", payload)
	defer server.Close()

	dir, _ := u.dataDir()
	updates := filepath.Join(dir, "updates")
	mustMkdirAll(t, updates)
	writeFileAt(t, filepath.Join(updates, "Veda-0.1.0-macos-arm64.dmg"), []byte("stale"), time.Now().Add(-time.Hour))
	writeFileAt(t, filepath.Join(updates, "Veda-0.1.0-macos-arm64.dmg"+downloadPartSuffix), []byte("half"), time.Now())

	u.Check()
	final := waitForUpdateStatus(t, u, updateStatusReady, updateStatusError)
	if final.Status != updateStatusReady {
		t.Fatalf("status = %q, error = %q", final.Status, final.Error)
	}

	got := namesIn(t, updates)
	if len(got) != 1 || got[0] != "Veda-0.2.0-macos-arm64.dmg" {
		t.Fatalf("下载后目录里剩 %v，want [Veda-0.2.0-macos-arm64.dmg]", got)
	}
}

// TestCleanupUpdateWorkDirsRemovesOnlyStaleOnes 验证只清理超过一小时的助手临时目录，
// 正在等待应用退出的那一个不会被误删。
func TestCleanupUpdateWorkDirsRemovesOnlyStaleOnes(t *testing.T) {
	temp := t.TempDir()
	now := time.Now()

	stale := filepath.Join(temp, updateWorkDirPrefix+"stale")
	fresh := filepath.Join(temp, updateWorkDirPrefix+"fresh")
	unrelated := filepath.Join(temp, "unrelated")
	for _, path := range []string{stale, fresh, unrelated} {
		mustMkdirAll(t, path)
	}
	writeFileAt(t, filepath.Join(stale, "apply.sh"), []byte("#!/bin/sh\n"), now.Add(-3*time.Hour))
	if err := os.Chtimes(stale, now.Add(-3*time.Hour), now.Add(-3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fresh, now, now); err != nil {
		t.Fatal(err)
	}

	cleanupUpdateWorkDirs(temp, now)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("过期助手目录应被删除，err = %v", err)
	}
	for _, path := range []string{fresh, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s 不应被删除：%v", filepath.Base(path), err)
		}
	}
}

// TestUpdateScriptsCleanUpDownloads 检查两个安装助手脚本都带着清理动作，并且模板占位符
// 已经全部替换。
func TestUpdateScriptsCleanUpDownloads(t *testing.T) {
	darwin := darwinUpdateScriptBody(4242, "/Applications/Veda.app", "/tmp/veda-update-1", "/Users/x/.veda/updates/Veda-0.3.0-macos-arm64.dmg")
	for _, want := range []string{"trap cleanup EXIT", `rm -f "$ARCHIVE"`, `rm -rf "$WORK"`, "4242"} {
		if !strings.Contains(darwin, want) {
			t.Errorf("macOS 脚本缺少 %q", want)
		}
	}
	if strings.Contains(darwin, "{{") {
		t.Errorf("macOS 脚本里还有未替换的占位符：\n%s", darwin)
	}

	windows := windowsUpdateScriptBody(4242, `C:\Users\x\.veda\updates\Veda-0.3.0-windows-x64-setup.exe`, `C:\Program Files\Veda\Veda.exe`)
	for _, want := range []string{":cleanup", `del /f /q "%INSTALLER%"`, "set /a TRIES=0", "4242"} {
		if !strings.Contains(windows, want) {
			t.Errorf("Windows 脚本缺少 %q", want)
		}
	}
	if strings.Contains(windows, "{{") {
		t.Errorf("Windows 脚本里还有未替换的占位符：\n%s", windows)
	}
	if !strings.Contains(windows, "\r\n") {
		t.Error("Windows 脚本应当使用 CRLF 换行")
	}
}

// TestDarwinUpdateScriptIsValidShell 用 sh -n 做一次语法检查，避免模板改坏后只在真实更新时
// 才暴露出来。
func TestDarwinUpdateScriptIsValidShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上没有 /bin/sh")
	}
	script := filepath.Join(t.TempDir(), "apply.sh")
	body := darwinUpdateScriptBody(1, "/Applications/Veda.app", "/tmp/veda-update-x", "/tmp/Veda.dmg")
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/bin/sh", "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("脚本语法检查失败：%v\n%s", err, out)
	}
}

// darwinApplyFixture 搭好一次安装助手脚本的运行环境：目标 .app、待安装的 zip 包、助手
// 临时目录，以及不会真去启动应用或操作磁盘映像的 open/hdiutil 桩。
type darwinApplyFixture struct {
	tmp     string
	app     string
	archive string
	work    string
	bin     string
	opened  string
}

func newDarwinApplyFixture(t *testing.T, bundle string, fill func(src string)) *darwinApplyFixture {
	t.Helper()
	f := &darwinApplyFixture{tmp: t.TempDir()}

	f.app = filepath.Join(f.tmp, "Veda.app")
	mustMkdirAll(t, f.app)
	writeFileAt(t, filepath.Join(f.app, "old-marker"), []byte("old"), time.Now())

	src := filepath.Join(f.tmp, bundle)
	mustMkdirAll(t, src)
	fill(src)

	updates := filepath.Join(f.tmp, "updates")
	mustMkdirAll(t, updates)
	f.archive = filepath.Join(updates, "Veda-0.9.9-macos-arm64.app.zip")
	if out, err := exec.Command("ditto", "-c", "-k", "--keepParent", src, f.archive).CombinedOutput(); err != nil {
		t.Fatalf("打包测试压缩包失败：%v\n%s", err, out)
	}

	f.work = filepath.Join(f.tmp, "work")
	mustMkdirAll(t, f.work)

	f.bin = filepath.Join(f.tmp, "bin")
	mustMkdirAll(t, f.bin)
	f.opened = filepath.Join(f.tmp, "opened.txt")
	openStub := "#!/bin/sh\necho \"$1\" >> '" + f.opened + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(f.bin, "open"), []byte(openStub), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.bin, "hdiutil"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return f
}

// run 执行助手脚本并返回退出码。PID 用一个不可能存在的值，脚本不会等待任何进程。
func (f *darwinApplyFixture) run(t *testing.T) int {
	t.Helper()
	script := filepath.Join(f.work, "apply.sh")
	if err := os.WriteFile(script, []byte(darwinUpdateScriptBody(999999, f.app, f.work, f.archive)), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", script)
	cmd.Env = append(os.Environ(), "PATH="+f.bin+":"+os.Getenv("PATH"))

	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	t.Fatalf("执行助手脚本失败：%v", err)
	return -1
}

// assertCleanedUp 断言安装包、助手临时目录与备份目录都被回收。
func (f *darwinApplyFixture) assertCleanedUp(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(f.archive); !os.IsNotExist(err) {
		t.Errorf("安装包应被删除，err = %v", err)
	}
	if _, err := os.Stat(f.work); !os.IsNotExist(err) {
		t.Errorf("助手临时目录应被删除，err = %v", err)
	}
	if _, err := os.Stat(f.app + ".old"); !os.IsNotExist(err) {
		t.Errorf("备份目录应被删除，err = %v", err)
	}
}

// TestDarwinUpdateScriptAppliesAndCleansUp 跑一遍真实的助手脚本（压缩包那条分支），确认
// 应用被替换、脚本被删除后仍能收尾，并且安装包不会留在本地。
func TestDarwinUpdateScriptAppliesAndCleansUp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("只在 macOS 上验证安装助手脚本")
	}
	f := newDarwinApplyFixture(t, "Veda-0.9.9.app", func(src string) {
		writeFileAt(t, filepath.Join(src, "new-marker"), []byte("new"), time.Now())
	})

	if code := f.run(t); code != 0 {
		t.Fatalf("助手脚本退出码 = %d，want 0", code)
	}

	f.assertCleanedUp(t)
	if _, err := os.Stat(filepath.Join(f.app, "new-marker")); err != nil {
		t.Errorf("应用没有被替换成新版本：%v", err)
	}
	if _, err := os.Stat(filepath.Join(f.app, "old-marker")); !os.IsNotExist(err) {
		t.Errorf("旧版本内容应被覆盖，err = %v", err)
	}
	raw, err := os.ReadFile(f.opened)
	if err != nil {
		t.Fatalf("替换完成后应重新打开应用：%v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != f.app {
		t.Errorf("open 收到 %q，want %q", got, f.app)
	}
}

// TestDarwinUpdateScriptCleansUpAfterFailure 验证压缩包里没有 .app 时脚本会失败退出，
// 但安装包与临时目录照样被清掉——这是这次修复的重点。
func TestDarwinUpdateScriptCleansUpAfterFailure(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("只在 macOS 上验证安装助手脚本")
	}
	// 压缩包里没有 .app：脚本会失败退出，但清理照样要执行。
	f := newDarwinApplyFixture(t, "Veda-0.9.9-payload", func(src string) {
		writeFileAt(t, filepath.Join(src, "readme.txt"), []byte("no bundle here"), time.Now())
	})

	if code := f.run(t); code == 0 {
		t.Fatal("压缩包里没有 .app，助手脚本应当失败退出")
	}

	f.assertCleanedUp(t)
	if _, err := os.Stat(filepath.Join(f.app, "old-marker")); err != nil {
		t.Errorf("安装失败时不应改动原应用：%v", err)
	}
	if _, err := os.Stat(f.opened); !os.IsNotExist(err) {
		t.Errorf("安装失败时不应重新打开应用，err = %v", err)
	}
}
