package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenameEntry(t *testing.T) {
	dir := t.TempDir()

	folder, err := createFolder(dir, "old-folder")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := renameEntry(folder.Path, "new-folder")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "new-folder" || !renamed.IsDir {
		t.Fatalf("got %+v", renamed)
	}
	if _, err := os.Stat(renamed.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folder.Path); !os.IsNotExist(err) {
		t.Fatalf("old folder should be gone: %v", err)
	}

	same, err := renameEntry(renamed.Path, "new-folder")
	if err != nil {
		t.Fatal(err)
	}
	if same.Path != renamed.Path {
		t.Fatalf("same name should be no-op: %s", same.Path)
	}

	doc, err := createMarkdown(dir, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	hello, err := renameEntry(doc.Path, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if hello.Name != "hello.md" || hello.Kind != KindMarkdown {
		t.Fatalf("preserve extension: %+v", hello)
	}

	txt, err := renameEntry(hello.Path, "readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if txt.Name != "readme.txt" {
		t.Fatalf("explicit extension: %s", txt.Name)
	}

	taken, err := createFolder(dir, "taken")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := renameEntry(renamed.Path, taken.Name); err == nil {
		t.Fatal("expected conflict")
	}

	nested, err := createMarkdown(renamed.Path, "child.md")
	if err != nil {
		t.Fatal(err)
	}
	moved, err := renameEntry(renamed.Path, "moved")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(moved.Path, "child.md")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("nested file should move with folder: %v", err)
	}
	if _, err := os.Stat(nested.Path); !os.IsNotExist(err) {
		t.Fatalf("old nested path should be gone: %v", err)
	}

	if _, err := renameEntry(moved.Path, "bad/name"); err == nil {
		t.Fatal("expected invalid name")
	}
}
