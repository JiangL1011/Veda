package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"veda/internal/model"
)

func TestRewriteConfigPath(t *testing.T) {
	got, ok := rewriteConfigPath("notes/old.md", "notes/old.md", "notes/new.md")
	if !ok || got != "notes/new.md" {
		t.Fatalf("file key: got %q ok=%v", got, ok)
	}
	got, ok = rewriteConfigPath("notes/old/a.md", "notes/old", "notes/renamed")
	if !ok || got != "notes/renamed/a.md" {
		t.Fatalf("dir prefix: got %q ok=%v", got, ok)
	}
	got, ok = rewriteConfigPath("notes/older/a.md", "notes/old", "notes/renamed")
	if ok {
		t.Fatalf("should not rewrite sibling prefix: %q", got)
	}
}

func TestRewriteVedaConfigsFileAndDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	ws := t.TempDir()
	oldDir := filepath.Join(ws, "notes")
	oldFile := filepath.Join(oldDir, "old.md")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldFile, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := &LockStore{}
	if err := store.Save(ws, oldFile, false); err != nil {
		t.Fatal(err)
	}
	sess := &SessionStore{}
	if err := sess.Save(&model.Session{Last: &model.OpenTarget{Kind: "directory", Path: ws, File: oldFile}}); err != nil {
		t.Fatal(err)
	}
	fileID := mustIndexID(t, oldFile)

	newFile := filepath.Join(oldDir, "new.md")
	if err := os.Rename(oldFile, newFile); err != nil {
		t.Fatal(err)
	}
	if err := rewriteVedaConfigs(oldFile, newFile, []string{ws}); err != nil {
		t.Fatal(err)
	}

	lock, err := store.Get(ws, newFile)
	if err != nil {
		t.Fatal(err)
	}
	if !lock.Found || lock.Editing {
		t.Fatalf("file lock should follow rename: %+v", lock)
	}
	loaded, err := sess.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Last == nil || filepath.ToSlash(loaded.Last.File) != filepath.ToSlash(newFile) {
		t.Fatalf("session file should follow rename: %+v", loaded.Last)
	}

	raw := mustWorkspaceFileBytes(t, ws, "locks.json")
	var doc fileLocksDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Files) != 1 {
		t.Fatalf("rename should keep exactly one lock entry: %s", raw)
	}
	if doc.Files[fileID] != lockModeReadonly {
		t.Fatalf("lock should follow the file id: %s", raw)
	}
	if id := mustIndexID(t, newFile); id != fileID {
		t.Fatalf("renamed file should keep its id: %s vs %s", id, fileID)
	}

	newDir := filepath.Join(ws, "docs")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(newDir, "new.md")
	if err := rewriteVedaConfigs(oldDir, newDir, []string{ws}); err != nil {
		t.Fatal(err)
	}

	lock, err = store.Get(ws, moved)
	if err != nil {
		t.Fatal(err)
	}
	if !lock.Found || lock.Editing {
		t.Fatalf("dir rename should keep nested lock: %+v", lock)
	}
	loaded, err = sess.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Last == nil || filepath.ToSlash(loaded.Last.File) != filepath.ToSlash(moved) {
		t.Fatalf("session file should follow dir rename: %+v", loaded.Last)
	}
}
