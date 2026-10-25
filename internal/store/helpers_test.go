package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// withTempHome 把 HOME 指到临时目录，隔离 ~/.veda 的读写。
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
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

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, raw)
	}
	return out
}

func mustReadFileIndex(t *testing.T) map[string]string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, dataDirName, fileIndexFileName))
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
	if !isUUID(id) {
		t.Fatalf("%s should have a uuid in file-index.json, got %q", abs, id)
	}
	return id
}
