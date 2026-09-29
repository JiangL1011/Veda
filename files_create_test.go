package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateFolderAndMarkdown(t *testing.T) {
	dir := t.TempDir()

	first, err := createFolder(dir, "新建文件夹")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "新建文件夹" || !first.IsDir {
		t.Fatalf("got %+v", first)
	}

	second, err := createFolder(dir, "新建文件夹")
	if err != nil {
		t.Fatal(err)
	}
	if second.Name != "新建文件夹 2" {
		t.Fatalf("unique folder name: %s", second.Name)
	}

	doc, err := createMarkdown(dir, "未命名.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.IsDir || doc.Kind != KindMarkdown || doc.Name != "未命名.md" {
		t.Fatalf("got %+v", doc)
	}
	raw, err := os.ReadFile(doc.Path)
	if err != nil || string(raw) != "" {
		t.Fatalf("empty markdown: %v %q", err, raw)
	}

	doc2, err := createMarkdown(dir, "未命名")
	if err != nil {
		t.Fatal(err)
	}
	if doc2.Name != "未命名 2.md" {
		t.Fatalf("unique markdown name: %s", doc2.Name)
	}

	nested, err := createMarkdown(first.Path, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(nested.Path) != first.Path {
		t.Fatalf("created under hovered dir: %s vs %s", nested.Path, first.Path)
	}
}
