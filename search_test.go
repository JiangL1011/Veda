package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchLiteralModes(t *testing.T) {
	content := "Foo foo FOO\nfood\nbar Foo"
	hit, err := searchContent(content, "Foo", SearchOptions{CaseSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := matchCount(hit); n != 2 {
		t.Fatalf("case sensitive Foo: %d", n)
	}

	hit, err = searchContent(content, "foo", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n := matchCount(hit); n != 5 {
		t.Fatalf("case insensitive foo should hit Foo/foo/FOO/food/Foo, got %d", n)
	}

	hit, err = searchContent(content, "foo", SearchOptions{WholeWord: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := matchCount(hit); n != 4 {
		t.Fatalf("whole word foo should skip food, got %d", n)
	}
}

func TestSearchChineseWholeWord(t *testing.T) {
	content := "搜索工作区\n搜索\n可搜索"
	hit, err := searchContent(content, "搜索", SearchOptions{WholeWord: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := matchCount(hit); n != 1 {
		t.Fatalf("whole-word 搜索 should only match the standalone token, got %d", n)
	}
	m := hit.Files[0].Matches[0]
	if m.Line != 2 || m.Column != 1 {
		t.Fatalf("expected line 2 col 1, got %d:%d", m.Line, m.Column)
	}
}

func TestSearchRegexAndInvalid(t *testing.T) {
	content := "abc aXc a1c"
	hit, err := searchContent(content, `a[bX]c`, SearchOptions{UseRegex: true, CaseSensitive: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := matchCount(hit); n != 2 {
		t.Fatalf("regex a[bX]c should hit abc and aXc, got %d", n)
	}

	hit, err = searchContent("abcXabc", `abc`, SearchOptions{UseRegex: true, WholeWord: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := matchCount(hit); n != 0 {
		t.Fatalf("regex whole word abc inside abcXabc: %d", n)
	}

	if _, err := searchContent(content, `(`, SearchOptions{UseRegex: true}); err == nil {
		t.Fatal("invalid regex should error")
	}
}

func TestSearchContextAndColumns(t *testing.T) {
	content := "one\ntarget here\nthree"
	hit, err := searchContent(content, "target", SearchOptions{ContextLines: 1})
	if err != nil {
		t.Fatal(err)
	}
	m := hit.Files[0].Matches[0]
	if m.Line != 2 || m.Column != 1 {
		t.Fatalf("locate: %d:%d", m.Line, m.Column)
	}
	if m.LineText != "target here" {
		t.Fatalf("line text: %q", m.LineText)
	}
	if len(m.Before) != 1 || m.Before[0] != "one" {
		t.Fatalf("before: %#v", m.Before)
	}
	if len(m.After) != 1 || m.After[0] != "three" {
		t.Fatalf("after: %#v", m.After)
	}

	hit, err = searchContent("中文测试", "测试", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	m = hit.Files[0].Matches[0]
	if m.Column != 3 {
		t.Fatalf("rune column should be 3, got %d", m.Column)
	}
}

func TestSearchTruncation(t *testing.T) {
	content := "a a a a a"
	hit, err := searchContent(content, "a", SearchOptions{MaxMatches: 2, MaxPerFile: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !hit.Truncated {
		t.Fatal("expected truncation")
	}
	if matchCount(hit) != 2 {
		t.Fatalf("got %d matches", matchCount(hit))
	}
}

func TestSearchWorkspaceSkipsAndTextKinds(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "note.md"), "hello markdown")
	mustWrite(t, filepath.Join(root, "data.json"), `{"hello": 1}`)
	mustWrite(t, filepath.Join(root, "skip.png"), "hello hidden in image")
	mustWrite(t, filepath.Join(root, "node_modules", "lib.js"), "hello from vendor")
	mustWrite(t, filepath.Join(root, ".hidden", "secret.md"), "hello hidden dir")
	outside := t.TempDir()
	mustWrite(t, filepath.Join(outside, "escape.md"), "hello outside")

	hit, err := searchWorkspace(context.Background(), root, "hello", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hit.Files) != 2 {
		t.Fatalf("expected md+json, got %+v", names(hit))
	}
	kinds := map[string]string{}
	for _, f := range hit.Files {
		kinds[filepath.Base(f.Path)] = f.Kind
		if f.RelPath == "" || filepath.IsAbs(f.RelPath) {
			t.Fatalf("rel path should be workspace-relative, got %q", f.RelPath)
		}
	}
	if kinds["note.md"] != KindMarkdown || kinds["data.json"] != KindText {
		t.Fatalf("kinds: %#v", kinds)
	}

	link := filepath.Join(root, "outside.md")
	if err := os.Symlink(filepath.Join(outside, "escape.md"), link); err == nil {
		hit, err = searchWorkspace(context.Background(), root, "hello", SearchOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range hit.Files {
			if filepath.Base(f.Path) == "outside.md" {
				t.Fatal("should not follow symlink out of workspace")
			}
		}
	}
}

func TestSearchWorkspaceCancel(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.md"), "needle")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := searchWorkspace(ctx, root, "needle", SearchOptions{}); err == nil {
		t.Fatal("expected canceled")
	}
}

func TestSearchWorkspaceRequiresDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "only.md")
	mustWrite(t, file, "x")
	if _, err := searchWorkspace(context.Background(), file, "x", SearchOptions{}); err == nil {
		t.Fatal("expected error")
	}
}

func matchCount(hit *SearchResult) int {
	if hit == nil || len(hit.Files) == 0 {
		return 0
	}
	n := 0
	for _, f := range hit.Files {
		n += len(f.Matches)
	}
	return n
}

func names(hit *SearchResult) []string {
	var out []string
	for _, f := range hit.Files {
		out = append(out, filepath.Base(f.Path))
	}
	return out
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
