package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAppServiceRenameRewritesConfigs(t *testing.T) {
	withTempHome(t)

	s := newTestService(t)
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
