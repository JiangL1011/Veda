package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteEntry(t *testing.T) {
	dir := t.TempDir()

	doc, err := createMarkdown(dir, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteEntry(doc.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(doc.Path); !os.IsNotExist(err) {
		t.Fatalf("file should be gone: %v", err)
	}

	folder, err := createFolder(dir, "keep")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := createMarkdown(folder.Path, "child.md")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := createFolder(folder.Path, "sub")
	if err != nil {
		t.Fatal(err)
	}
	deep, err := createMarkdown(sub.Path, "deep.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteEntry(folder.Path); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{folder.Path, nested.Path, sub.Path, deep.Path} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone: %v", gone, err)
		}
	}

	if err := deleteEntry(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected missing path to fail")
	}
}
