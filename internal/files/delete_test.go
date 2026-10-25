package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteEntry(t *testing.T) {
	dir := t.TempDir()

	doc, err := CreateDocument(dir, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteEntry(doc.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(doc.Path); !os.IsNotExist(err) {
		t.Fatalf("file should be gone: %v", err)
	}

	folder, err := CreateFolder(dir, "keep")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := CreateDocument(folder.Path, "child.md")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := CreateFolder(folder.Path, "sub")
	if err != nil {
		t.Fatal(err)
	}
	deep, err := CreateDocument(sub.Path, "deep.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteEntry(folder.Path); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{folder.Path, nested.Path, sub.Path, deep.Path} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone: %v", gone, err)
		}
	}

	if err := DeleteEntry(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected missing path to fail")
	}
}
