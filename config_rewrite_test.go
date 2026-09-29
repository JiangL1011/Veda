package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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

	store := &lockStore{}
	if err := store.save(ws, oldFile, false); err != nil {
		t.Fatal(err)
	}
	sess := &sessionStore{}
	if err := sess.Save(&Session{Last: &OpenTarget{Kind: "directory", Path: ws, File: oldFile}}); err != nil {
		t.Fatal(err)
	}

	newFile := filepath.Join(oldDir, "new.md")
	if err := os.Rename(oldFile, newFile); err != nil {
		t.Fatal(err)
	}
	if err := rewriteVedaConfigs(oldFile, newFile, []string{ws}); err != nil {
		t.Fatal(err)
	}

	lock, err := store.get(ws, newFile)
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
	if doc.Files["notes/old.md"] != "" {
		t.Fatalf("old relative key should be gone: %s", raw)
	}
	if doc.Files["notes/new.md"] != lockModeReadonly {
		t.Fatalf("new relative key missing: %s", raw)
	}

	newDir := filepath.Join(ws, "docs")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(newDir, "new.md")
	if err := rewriteVedaConfigs(oldDir, newDir, []string{ws}); err != nil {
		t.Fatal(err)
	}

	lock, err = store.get(ws, moved)
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

func TestAppServiceRenameRewritesConfigs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	s := newAppService()
	ws := t.TempDir()
	oldDir := filepath.Join(ws, "notes")
	oldFile := filepath.Join(oldDir, "old.md")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldFile, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	s.allow(ws)
	if err := s.SaveFileLock(ws, oldFile, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSession(context.Background(), "directory", ws, oldFile); err != nil {
		t.Fatal(err)
	}

	renamed, err := s.Rename(oldFile, "new.md")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := s.GetFileLock(ws, renamed.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !lock.Found || lock.Editing {
		t.Fatalf("rename should keep readonly lock: %+v", lock)
	}
	sess, err := s.GetSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Last == nil || filepath.ToSlash(sess.Last.File) != filepath.ToSlash(renamed.Path) {
		t.Fatalf("session should record renamed file: %+v", sess.Last)
	}

	folder, err := s.Rename(oldDir, "docs")
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(folder.Path, "new.md")
	lock, err = s.GetFileLock(ws, child)
	if err != nil {
		t.Fatal(err)
	}
	if !lock.Found || lock.Editing {
		t.Fatalf("folder rename should keep nested lock: %+v", lock)
	}
	sess, err = s.GetSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Last == nil || filepath.ToSlash(sess.Last.File) != filepath.ToSlash(child) {
		t.Fatalf("session should follow folder rename: %+v", sess.Last)
	}

	standalone := filepath.Join(t.TempDir(), "alone.md")
	if err := os.WriteFile(standalone, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveFileLock("", standalone, false); err != nil {
		t.Fatal(err)
	}
	movedAlone, err := s.Rename(standalone, "renamed.md")
	if err != nil {
		t.Fatal(err)
	}
	lock, err = s.GetFileLock("", movedAlone.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !lock.Found || lock.Editing {
		t.Fatalf("global lock should follow standalone rename: %+v", lock)
	}
}
