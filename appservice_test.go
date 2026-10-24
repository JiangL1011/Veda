package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestReadTextAlwaysReadsCurrentDiskContent(t *testing.T) {
	withTempHome(t)
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := newAppService()
	first, err := service.ReadText(path)
	if err != nil || first.Content != "first" {
		t.Fatalf("first read: %+v, %v", first, err)
	}

	if err := os.WriteFile(path, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := service.ReadText(path)
	if err != nil || second.Content != "second" {
		t.Fatalf("second read should come from disk: %+v, %v", second, err)
	}
}

// 绑定的调用号同时写在 Go（RegisterBindingMethodID）和生成的 TS 里，
// 两边不一致时前端会报 "unknown bound method id"，这里守住这一对。
func assertBindingID(t *testing.T, name string, want uint32) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("frontend", "bindings", "veda", "appservice.ts"))
	if err != nil {
		t.Skipf("generated bindings not available: %v", err)
	}
	pattern := regexp.MustCompile(`export function ` + name + `\([^)]*\)[^{]*\{[^}]*\$Call\.ByID\((\d+)`)
	match := pattern.FindSubmatch(raw)
	if match == nil {
		t.Fatalf("%s binding not found in generated appservice.ts", name)
	}
	got, err := strconv.ParseUint(string(match[1]), 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(want) != got {
		t.Fatalf("%s binding id mismatch: Go has %d, frontend has %d", name, want, got)
	}
}

func TestCreateDocumentBindingIDMatchesFrontend(t *testing.T) {
	assertBindingID(t, "CreateDocument", createDocumentBindingID)
}

func TestMoveBindingIDMatchesFrontend(t *testing.T) {
	assertBindingID(t, "Move", moveBindingID)
}

func TestDataMigrationBindingIDsMatchFrontend(t *testing.T) {
	assertBindingID(t, "GetDataMigrationState", getDataMigrationStateBindingID)
	assertBindingID(t, "RetryDataMigration", retryDataMigrationBindingID)
}
