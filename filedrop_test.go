package main

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestUniqueChildPathParen(t *testing.T) {
	dir := t.TempDir()

	full, name, err := uniqueChildPathParen(dir, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if name != "note.md" || full != filepath.Join(dir, "note.md") {
		t.Fatalf("got %s %s", name, full)
	}

	writeFile(t, filepath.Join(dir, "note.md"), "a")
	_, name, err = uniqueChildPathParen(dir, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if name != "note(1).md" {
		t.Fatalf("first duplicate: %s", name)
	}

	writeFile(t, filepath.Join(dir, "note(1).md"), "b")
	_, name, err = uniqueChildPathParen(dir, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if name != "note(2).md" {
		t.Fatalf("second duplicate: %s", name)
	}

	// 无后缀的名字直接追加序号。
	writeFile(t, filepath.Join(dir, "README"), "x")
	_, name, err = uniqueChildPathParen(dir, "README")
	if err != nil {
		t.Fatal(err)
	}
	if name != "README(1)" {
		t.Fatalf("extensionless duplicate: %s", name)
	}
}

func TestImportPathsIntoCopyKeepsSource(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "note.md"), "hello")

	outcome := importPathsInto([]string{filepath.Join(src, "note.md")}, root, root, false)
	if len(outcome.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", outcome.Errors)
	}
	if len(outcome.Imported) != 1 || outcome.Action != dragDropCopy {
		t.Fatalf("got %+v", outcome)
	}
	entry := outcome.Imported[0]
	if entry.Name != "note.md" || entry.Kind != KindMarkdown || entry.IsDir {
		t.Fatalf("entry: %+v", entry)
	}
	if readFile(t, entry.Path) != "hello" {
		t.Fatal("copied content mismatch")
	}
	if readFile(t, filepath.Join(src, "note.md")) != "hello" {
		t.Fatal("copy should keep the source")
	}

	// 再次导入重名文件时自动变成 note(1).md。
	second := importPathsInto([]string{filepath.Join(src, "note.md")}, root, root, false)
	if len(second.Imported) != 1 || second.Imported[0].Name != "note(1).md" {
		t.Fatalf("second import: %+v", second.Imported)
	}
}

func TestImportPathsIntoMoveRemovesSource(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	from := filepath.Join(src, "draft.txt")
	writeFile(t, from, "draft")

	outcome := importPathsInto([]string{from}, root, root, true)
	if len(outcome.Errors) != 0 || len(outcome.Imported) != 1 {
		t.Fatalf("got %+v", outcome)
	}
	if outcome.Action != dragDropMove {
		t.Fatalf("action: %s", outcome.Action)
	}
	if readFile(t, outcome.Imported[0].Path) != "draft" {
		t.Fatal("moved content mismatch")
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Fatalf("source should be gone, got %v", err)
	}
}

func TestImportPathsIntoFolder(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "notes", "a.md"), "a")
	writeFile(t, filepath.Join(src, "notes", "sub", "b.md"), "b")

	outcome := importPathsInto([]string{filepath.Join(src, "notes")}, root, root, false)
	if len(outcome.Errors) != 0 || len(outcome.Imported) != 1 {
		t.Fatalf("got %+v", outcome)
	}
	entry := outcome.Imported[0]
	if !entry.IsDir || entry.Kind != KindDir {
		t.Fatalf("entry: %+v", entry)
	}
	if readFile(t, filepath.Join(entry.Path, "sub", "b.md")) != "b" {
		t.Fatal("nested file missing after copy")
	}
}

func TestImportPathsIntoRejectsFolderInsideItself(t *testing.T) {
	root := t.TempDir()
	inner := filepath.Join(root, "notes")
	writeFile(t, filepath.Join(inner, "a.md"), "a")

	outcome := importPathsInto([]string{inner}, inner, root, false)
	if len(outcome.Imported) != 0 || len(outcome.Errors) != 1 {
		t.Fatalf("got %+v", outcome)
	}
}

func TestImportPathsIntoCollectsFailures(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	good := filepath.Join(src, "ok.md")
	writeFile(t, good, "ok")

	outcome := importPathsInto([]string{filepath.Join(src, "missing.md"), good}, root, root, false)
	if len(outcome.Errors) != 1 || len(outcome.Imported) != 1 {
		t.Fatalf("got %+v", outcome)
	}
	if outcome.Imported[0].Name != "ok.md" {
		t.Fatalf("entry: %+v", outcome.Imported[0])
	}
}

func TestResolveDropDir(t *testing.T) {
	svc := &AppService{}
	root := t.TempDir()
	sub := filepath.Join(root, "notes")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()

	if got := svc.resolveDropDir(root, ""); got != root {
		t.Fatalf("empty hint should use the workspace root, got %s", got)
	}
	if got := svc.resolveDropDir(root, sub); got != sub {
		t.Fatalf("inside hint should win, got %s", got)
	}
	if got := svc.resolveDropDir(root, outside); got != root {
		t.Fatalf("outside hint should fall back, got %s", got)
	}
	if got := svc.resolveDropDir(root, filepath.Join(root, "missing")); got != root {
		t.Fatalf("missing hint should fall back, got %s", got)
	}
}

func TestIsSameOrInside(t *testing.T) {
	sep := string(os.PathSeparator)
	root := filepath.Join(sep, "a", "b")
	if !isSameOrInside(root, root) {
		t.Fatal("same path")
	}
	if !isSameOrInside(root, filepath.Join(root, "c", "d")) {
		t.Fatal("nested path")
	}
	if isSameOrInside(root, filepath.Join(sep, "a", "bc")) {
		t.Fatal("sibling prefix must not match")
	}
}

func TestDragDropModeFallsBackToGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	svc := &AppService{settings: &settingsStore{}}

	global := DefaultSettings()
	global.General.DragDropMode = dragDropMove
	if _, err := svc.settings.save("global", "", global); err != nil {
		t.Fatal(err)
	}

	// 没有工作区覆盖层时沿用全局设置。
	plain := t.TempDir()
	if mode := svc.dragDropMode(plain); mode != dragDropMove {
		t.Fatalf("workspace without its own settings should follow global, got %s", mode)
	}

	// 工作区把自己的设置改成复制后，只影响这个工作区。
	ws := t.TempDir()
	scoped := DefaultSettings()
	scoped.General.DragDropMode = dragDropCopy
	if _, err := svc.settings.save("workspace", ws, scoped); err != nil {
		t.Fatal(err)
	}
	if mode := svc.dragDropMode(ws); mode != dragDropCopy {
		t.Fatalf("workspace override should win, got %s", mode)
	}
	if mode := svc.dragDropMode(plain); mode != dragDropMove {
		t.Fatalf("other workspaces should stay on global, got %s", mode)
	}
}

func TestApplyFileDropRecordsOutcome(t *testing.T) {
	svc := &AppService{
		roots:        map[string]struct{}{},
		files:        map[string]struct{}{},
		pendingDrops: map[uint]*pendingFileDrop{},
	}
	root := t.TempDir()
	src := t.TempDir()
	from := filepath.Join(src, "note.md")
	writeFile(t, from, "note")

	outcome := svc.applyFileDrop(nil, []string{from}, root, root, false)
	if len(outcome.Errors) != 0 || len(outcome.Imported) != 1 {
		t.Fatalf("got %+v", outcome)
	}
	if !svc.isAllowed(outcome.Imported[0].Path) {
		t.Fatal("imported file should be allowed for later reads")
	}

	// 移动之后源文件不再需要单独放行。
	moved := svc.applyFileDrop(nil, []string{from}, root, root, true)
	if len(moved.Errors) != 0 || moved.Imported[0].Name != "note(1).md" {
		t.Fatalf("move: %+v", moved)
	}
	if svc.isAllowed(from) {
		t.Fatal("moved source should no longer be allowed")
	}
}

func TestResolveFileDropCancel(t *testing.T) {
	svc := &AppService{pendingDrops: map[uint]*pendingFileDrop{}}
	svc.rememberPendingDrop(7, &pendingFileDrop{files: []string{"/tmp/a"}, destDir: "/tmp", workspace: "/tmp"})
	if drop := svc.takePendingDrop(7); drop == nil {
		t.Fatal("pending drop should be recorded")
	}
	if drop := svc.takePendingDrop(7); drop != nil {
		t.Fatal("pending drop should be consumed once")
	}
}
