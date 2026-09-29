package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenTargetForPathClassifiesSupportedFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	note := filepath.Join(root, "NOTE.MD")
	if err := os.WriteFile(note, []byte("# note"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "image.png")
	if err := os.WriteFile(other, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := openTargetForPath(root, ""); got == nil || got.Kind != "directory" || got.Path != root {
		t.Fatalf("directory target = %+v", got)
	}
	if got := openTargetForPath(note, ""); got == nil || got.Kind != "file" || got.Path != note {
		t.Fatalf("file target = %+v", got)
	}
	if got := openTargetForPath(other, ""); got != nil {
		t.Fatalf("unsupported file target = %+v", got)
	}
	if got := openTargetForPath(filepath.Join(root, "missing.md"), ""); got != nil {
		t.Fatalf("missing file target = %+v", got)
	}
}

func TestOpenTargetsFromArgsResolvesRelativePathsAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	note := filepath.Join(root, "note.txt")
	if err := os.WriteFile(note, []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := openTargetsFromArgs(
		[]string{"veda", "note.txt", note, "--ignored", "missing.md"},
		root,
	)
	if len(got) != 1 {
		t.Fatalf("targets = %+v", got)
	}
	if got[0].Kind != "file" || got[0].Path != note {
		t.Fatalf("target = %+v", got[0])
	}
}

func TestExternalOpenCoordinatorConsumesSuppressionOnlyOnce(t *testing.T) {
	coordinator := newExternalOpenCoordinator(nil)
	path := filepath.Join(t.TempDir(), "note.md")
	coordinator.suppressFrameworkOpen(path)

	if !coordinator.consumeSuppressed(path) {
		t.Fatal("first matching framework event should be suppressed")
	}
	if coordinator.consumeSuppressed(path) {
		t.Fatal("suppression should be consumed")
	}
}
