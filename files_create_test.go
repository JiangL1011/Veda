package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func errorCodeOf(t *testing.T, err error) string {
	t.Helper()
	var coded *codedError
	if !errors.As(err, &coded) {
		t.Fatalf("expected coded error, got %v", err)
	}
	return coded.code
}

func TestCreateFolderAndDocument(t *testing.T) {
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

	doc, err := createDocument(dir, "未命名.md")
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

	// 不写后缀时按用户输入创建，不再自动补 .md。
	plain, err := createDocument(dir, "未命名")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Name != "未命名" {
		t.Fatalf("extension should be user-defined: %s", plain.Name)
	}

	// 重名时在主干后追加序号，后缀原样保留。
	plainAgain, err := createDocument(dir, "未命名")
	if err != nil {
		t.Fatal(err)
	}
	if plainAgain.Name != "未命名 2" {
		t.Fatalf("unique extensionless name: %s", plainAgain.Name)
	}

	nested, err := createDocument(first.Path, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(nested.Path) != first.Path {
		t.Fatalf("created under hovered dir: %s vs %s", nested.Path, first.Path)
	}
}

func TestCreateDocumentKeepsUserExtension(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name string
		kind string
	}{
		{"script.js", KindText},
		{"notes.txt", KindMarkdown},
		{"README", KindMarkdown},
		{".env", KindText},
		{"data.json", KindText},
	}
	for _, tc := range cases {
		entry, err := createDocument(dir, tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if entry.Name != tc.name || entry.Kind != tc.kind {
			t.Fatalf("%s: got %+v", tc.name, entry)
		}
	}
}

func TestCreateDocumentRejectsNonTextExtension(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"photo.jpg", "clip.mp4", "archive.zip", "font.ttf"} {
		entry, err := createDocument(dir, name)
		if err == nil {
			t.Fatalf("%s should be rejected, got %+v", name, entry)
		}
		if code := errorCodeOf(t, err); code != errCodeUnsupportedDocType {
			t.Fatalf("%s: unexpected code %q", name, code)
		}
		if _, statErr := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("%s should not be created: %v", name, statErr)
		}
	}
}
