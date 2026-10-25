package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const errCodeMoveIntoSelf = "move-into-self"

// withTempHome 把 HOME 指到临时目录，隔离 ~/.veda 的读写。
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// newTestService 创建一个不依赖 Wails 运行时的服务实例。
func newTestService(_ *testing.T) *AppService {
	return New(func() string { return "0.0.0" })
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadFileIndex(t *testing.T) map[string]string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".veda", "file-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	if err := json.Unmarshal(raw, &files); err != nil {
		t.Fatalf("file-index.json should be a path → id object: %v\n%s", err, raw)
	}
	return files
}

func mustIndexID(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	id := mustReadFileIndex(t)[abs]
	if id == "" {
		t.Fatalf("%s should have an id in file-index.json", abs)
	}
	return id
}

// errorCodeOf 从 "[code] 文案" 形式的错误里取出错误码。
func errorCodeOf(t *testing.T, err error) string {
	t.Helper()
	text := err.Error()
	if !strings.HasPrefix(text, "[") {
		t.Fatalf("expected coded error, got %v", err)
	}
	end := strings.IndexByte(text, ']')
	if end < 0 {
		t.Fatalf("expected coded error, got %v", err)
	}
	return text[1:end]
}
