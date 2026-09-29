package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestImportImageDataUsesWorkspaceAssetsAndUniqueNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	workspace := t.TempDir()
	documentDir := filepath.Join(workspace, "notes")
	if err := os.MkdirAll(documentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	document := filepath.Join(documentDir, "note.md")
	if err := os.WriteFile(document, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	service := newAppService()
	encoded := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nimage"))
	first, err := service.ImportImageData(workspace, "pic.png", "image/png", encoded)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.ImportImageData(workspace, "pic.png", "image/png", encoded)
	if err != nil {
		t.Fatal(err)
	}
	third, err := service.ImportImageData(workspace, "pic(1).png", "image/png", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "pic.png" || second.Name != "pic(1).png" || third.Name != "pic(2).png" {
		t.Fatalf("unexpected names: %q, %q, %q", first.Name, second.Name, third.Name)
	}
	if want := "assets/pic.png"; first.MarkdownPath != want {
		t.Fatalf("markdown path = %q, want workspace-relative %q", first.MarkdownPath, want)
	}
	if _, err := os.Stat(filepath.Join(workspace, "assets", "pic(1).png")); err != nil {
		t.Fatal(err)
	}
}

func TestImportImageDataUsesConfiguredGlobalDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	service := newAppService()
	settings := DefaultSettings()
	settings.General.ResourceDirectory = "media/images"
	if _, err := service.settings.save("global", "", settings); err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nimage"))
	asset, err := service.ImportImageData("", "clip", "image/png", encoded)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(home, ".veda", "media", "images", "clip.png")
	if asset.Path != expected {
		t.Fatalf("path = %q, want %q", asset.Path, expected)
	}
	if asset.MarkdownPath != filepath.ToSlash(expected) {
		t.Fatalf("markdown path = %q, want %q", asset.MarkdownPath, filepath.ToSlash(expected))
	}
}

func TestNormalizeResourceDirectoryRejectsEscapes(t *testing.T) {
	for _, value := range []string{"", ".", "../assets", "foo/../../assets", "/tmp/assets"} {
		if got := normalizeResourceDirectory(value); got != "assets" {
			t.Fatalf("normalizeResourceDirectory(%q) = %q", value, got)
		}
	}
	if got := normalizeResourceDirectory("media/images"); got != filepath.Join("media", "images") {
		t.Fatalf("nested resource directory = %q", got)
	}
}

func TestReadDirCanExposeConfiguredHiddenResourceDirectory(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, ".assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".other"), 0o755); err != nil {
		t.Fatal(err)
	}
	hidden, err := readDirEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 0 {
		t.Fatalf("hidden listing = %+v", hidden)
	}
	visible, err := readDirEntriesWithVisiblePath(root, assets)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].Path != assets {
		t.Fatalf("visible listing = %+v", visible)
	}
}

func TestResolveImagePathForWorkspaceAndSingleFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	service := newAppService()

	workspace := t.TempDir()
	noteDir := filepath.Join(workspace, "notes", "daily")
	asset := filepath.Join(workspace, "assets", "pic.png")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(asset), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(asset, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := service.ResolveImagePath(workspace, noteDir, "assets/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	if got != asset {
		t.Fatalf("workspace image = %q, want %q", got, asset)
	}

	project := t.TempDir()
	singleDir := filepath.Join(project, "docs", "daily")
	upwardAsset := filepath.Join(project, "assets", "nested", "pic.png")
	if err := os.MkdirAll(singleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(upwardAsset), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(upwardAsset, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = service.ResolveImagePath("", singleDir, "assets/nested/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	if got != upwardAsset {
		t.Fatalf("single-file upward image = %q, want %q", got, upwardAsset)
	}

	fallback, err := service.ResolveImagePath("", singleDir, "assets/missing.png")
	if err != nil {
		t.Fatal(err)
	}
	wantFallback := filepath.Join(home, ".veda", "assets", "missing.png")
	if fallback != wantFallback {
		t.Fatalf("single-file fallback = %q, want %q", fallback, wantFallback)
	}
}
