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

	doc, err := createDocument(dir, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	hello, err := renameEntry(doc.Path, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if hello.Name != "hello" || hello.Kind != KindText {
		t.Fatalf("text file may drop its extension: %+v", hello)
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

	nested, err := createDocument(renamed.Path, "child.md")
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

func TestRenameTextFileMayChangeExtension(t *testing.T) {
	dir := t.TempDir()

	doc, err := createDocument(dir, "note.md")
	if err != nil {
		t.Fatal(err)
	}
	// 文本文件可以在文本类型之间换后缀，也可以去掉后缀。
	for _, name := range []string{"note.MD", "note.txt", "note.js", "note"} {
		next, err := renameEntry(doc.Path, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if next.Name != name {
			t.Fatalf("%s: got %s", name, next.Name)
		}
		doc = next
	}

	// 未知后缀但只要内容是文本，也算文本文件。
	unknown := filepath.Join(dir, "notes.xyz")
	if err := os.WriteFile(unknown, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	converted, err := renameEntry(unknown, "notes.md")
	if err != nil {
		t.Fatal(err)
	}
	if converted.Name != "notes.md" || converted.Kind != KindMarkdown {
		t.Fatalf("text content with unknown extension: %+v", converted)
	}

	// 文本文件不允许改成非文本后缀。
	if _, err := renameEntry(converted.Path, "notes.jpg"); err == nil {
		t.Fatal("text file should not become an image")
	} else if code := errorCodeOf(t, err); code != errCodeUnsupportedDocType {
		t.Fatalf("unexpected code %q", code)
	}
	if _, err := os.Stat(converted.Path); err != nil {
		t.Fatalf("rejected rename must keep the file: %v", err)
	}
}

func TestRenameNonTextFileKeepsExtension(t *testing.T) {
	dir := t.TempDir()

	photo := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(photo, []byte{0xff, 0xd8, 0xff, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}

	// 只改文件名、不改后缀：允许。
	renamed, err := renameEntry(photo, "holiday.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "holiday.jpg" {
		t.Fatalf("got %s", renamed.Name)
	}

	// 不写后缀时自动补回原后缀，等同于只改文件名。
	kept, err := renameEntry(renamed.Path, "summer")
	if err != nil {
		t.Fatal(err)
	}
	if kept.Name != "summer.jpg" {
		t.Fatalf("extension should be preserved: %s", kept.Name)
	}

	// 改动非文本文件的后缀：拒绝。
	for _, name := range []string{"summer.png", "summer.txt"} {
		if _, err := renameEntry(kept.Path, name); err == nil {
			t.Fatalf("%s should be rejected", name)
		}
	}
	if _, err := os.Stat(kept.Path); err != nil {
		t.Fatalf("rejected rename must keep the file: %v", err)
	}

	// 无后缀的二进制文件同样不能加后缀，但其内容为文本时按文本处理。
	blob := filepath.Join(dir, "blob")
	if err := os.WriteFile(blob, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := renameEntry(blob, "blob.bin"); err == nil {
		t.Fatal("binary file without extension should not gain one")
	} else if code := errorCodeOf(t, err); code != errCodeNonTextExtension {
		t.Fatalf("unexpected code %q", code)
	}

	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	withExt, err := renameEntry(plain, "plain.md")
	if err != nil {
		t.Fatal(err)
	}
	if withExt.Name != "plain.md" {
		t.Fatalf("text file should gain an extension: %s", withExt.Name)
	}
}
