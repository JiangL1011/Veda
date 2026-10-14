package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Markdown 模式只对 Markdown 后缀生效；.txt 与无后缀文件都是普通文本。
func TestClassifyMarkdownOnlyForMarkdownExtensions(t *testing.T) {
	cases := []struct {
		name string
		kind string
	}{
		{"note.md", KindMarkdown},
		{"note.markdown", KindMarkdown},
		{"note.mdown", KindMarkdown},
		{"note.mdx", KindMarkdown},
		{"note.MD", KindMarkdown},
		{"notes.txt", KindText},
		{"notes.text", KindText},
		{"README", KindText},
	}
	for _, tc := range cases {
		if kind := classifyFile(tc.name, false); kind != tc.kind {
			t.Fatalf("%s: got %s, want %s", tc.name, kind, tc.kind)
		}
	}
}

// .txt 的内容即使写得像 Markdown，也按普通文本打开，内容原样读取。
func TestTextExtensionStaysPlainText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	body := "# 标题\n\n- 列表项\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	meta, err := fileMeta(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Kind != KindText {
		t.Fatalf("txt file should be plain text: %s", meta.Kind)
	}

	text, err := readTextFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if text.Kind != KindText || text.Content != body {
		t.Fatalf("got %+v", text)
	}
}

// 无后缀文件按内容嗅探：文本可编辑，二进制仍算未知类型。
func TestFileMetaSniffsExtensionlessContent(t *testing.T) {
	dir := t.TempDir()

	plain := filepath.Join(dir, "NOTES")
	if err := os.WriteFile(plain, []byte("# 标题\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta, err := fileMeta(plain)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Kind != KindText {
		t.Fatalf("extensionless text file: got %s", meta.Kind)
	}

	blob := filepath.Join(dir, "blob")
	if err := os.WriteFile(blob, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	blobMeta, err := fileMeta(blob)
	if err != nil {
		t.Fatal(err)
	}
	if blobMeta.Kind != KindUnknown {
		t.Fatalf("extensionless binary file: got %s", blobMeta.Kind)
	}
}
