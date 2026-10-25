package files

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

	outcome := ImportPathsInto([]string{filepath.Join(src, "note.md")}, root, false)
	if len(outcome.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", outcome.Errors)
	}
	entry := outcome.Entries[0]
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
	second := ImportPathsInto([]string{filepath.Join(src, "note.md")}, root, false)
	if len(second.Entries) != 1 || second.Entries[0].Name != "note(1).md" {
		t.Fatalf("second import: %+v", second.Entries)
	}
}

func TestImportPathsIntoMoveRemovesSource(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	from := filepath.Join(src, "draft.txt")
	writeFile(t, from, "draft")

	outcome := ImportPathsInto([]string{from}, root, true)
	if len(outcome.Errors) != 0 || len(outcome.Entries) != 1 {
		t.Fatalf("got %+v", outcome)
	}
	if readFile(t, outcome.Entries[0].Path) != "draft" {
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

	outcome := ImportPathsInto([]string{filepath.Join(src, "notes")}, root, false)
	if len(outcome.Errors) != 0 || len(outcome.Entries) != 1 {
		t.Fatalf("got %+v", outcome)
	}
	entry := outcome.Entries[0]
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

	outcome := ImportPathsInto([]string{inner}, inner, false)
	if len(outcome.Entries) != 0 || len(outcome.Errors) != 1 {
		t.Fatalf("got %+v", outcome)
	}
}

func TestImportPathsIntoCollectsFailures(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	good := filepath.Join(src, "ok.md")
	writeFile(t, good, "ok")

	outcome := ImportPathsInto([]string{filepath.Join(src, "missing.md"), good}, root, false)
	if len(outcome.Errors) != 1 || len(outcome.Entries) != 1 {
		t.Fatalf("got %+v", outcome)
	}
	if outcome.Entries[0].Name != "ok.md" {
		t.Fatalf("entry: %+v", outcome.Entries[0])
	}
}

func TestIsSameOrInside(t *testing.T) {
	sep := string(os.PathSeparator)
	root := filepath.Join(sep, "a", "b")
	if !IsSameOrInside(root, root) {
		t.Fatal("same path")
	}
	if !IsSameOrInside(root, filepath.Join(root, "c", "d")) {
		t.Fatal("nested path")
	}
	if IsSameOrInside(root, filepath.Join(sep, "a", "bc")) {
		t.Fatal("sibling prefix must not match")
	}
}
