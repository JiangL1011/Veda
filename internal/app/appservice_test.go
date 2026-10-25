package app

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
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

	service := newTestService(t)
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

// 绑定的调用号由 wails3 generate 按 "包路径.类型名.方法名" 的 FNV-1a 哈希算出，
// 运行时用同样的规则解析。两边必须一致，否则前端会报 "unknown bound method id"。
// 以前 AppService 在 package main 里，两边算出的包路径不同，只好在 init 里逐个
// 注册固定 ID；现在服务在 internal/app，规则天然一致，这里守住这一点。
func TestBindingIDsMatchGeneratedFrontend(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "frontend", "bindings", "veda", "internal", "app", "appservice.ts"))
	if err != nil {
		t.Skipf("generated bindings not available: %v", err)
	}
	pattern := regexp.MustCompile(`export function ([A-Za-z0-9_]+)\([^)]*\)[^{]*\{[^}]*\$Call\.ByID\((\d+)`)
	matches := pattern.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		t.Fatal("no bound methods found in generated appservice.ts")
	}

	pkgPath := reflect.TypeOf(AppService{}).PkgPath()
	for _, match := range matches {
		method := match[1]
		fqn := pkgPath + ".AppService." + method
		want := fnv32a(fqn)
		got, err := strconv.ParseUint(match[2], 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		if uint32(got) != want {
			t.Fatalf("%s binding id mismatch: Go hashes %q to %d, frontend has %d", method, fqn, want, got)
		}
	}
}

func fnv32a(value string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return h.Sum32()
}
