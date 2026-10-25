package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLockStoreWorkspaceAndGlobal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &LockStore{}
	ws := t.TempDir()
	note := filepath.Join(ws, "notes", "hello.md")
	if err := os.MkdirAll(filepath.Dir(note), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(note, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	missing, err := store.Get(ws, note)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Found || !missing.Editing {
		t.Fatalf("missing lock should open in edit mode: %+v", missing)
	}

	if err := store.Save(ws, note, false); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ws, note)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.Editing {
		t.Fatalf("workspace lock should be readonly: %+v", got)
	}

	raw := mustWorkspaceFileBytes(t, ws, "locks.json")
	var doc fileLocksDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Files[mustIndexID(t, note)] != lockModeReadonly {
		t.Fatalf("workspace locks should be keyed by file id, got %s", raw)
	}
	assertNoWorkspaceDotDir(t, ws)

	standalone := filepath.Join(t.TempDir(), "alone.md")
	if err := os.WriteFile(standalone, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Save("", standalone, true); err != nil {
		t.Fatal(err)
	}
	got, err = store.Get("", standalone)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || !got.Editing {
		t.Fatalf("standalone lock should be edit: %+v", got)
	}
	globalRaw, err := os.ReadFile(filepath.Join(home, ".veda", "locks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var globalDoc fileLocksDoc
	if err := json.Unmarshal(globalRaw, &globalDoc); err != nil {
		t.Fatal(err)
	}
	if globalDoc.Files[mustIndexID(t, standalone)] != lockModeEdit {
		t.Fatalf("global locks should be keyed by file id, got %s", globalRaw)
	}
}

func TestLockStoreRewriteRenamedFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &LockStore{}
	ws := t.TempDir()
	oldPath := filepath.Join(ws, "notes", "old.md")
	newPath := filepath.Join(ws, "notes", "new.md")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ws, oldPath, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if err := store.Rewrite(ws, oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	oldState, err := store.Get(ws, oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if oldState.Found {
		t.Fatal("old path should no longer have a lock")
	}
	newState, err := store.Get(ws, newPath)
	if err != nil {
		t.Fatal(err)
	}
	if !newState.Found || newState.Editing {
		t.Fatalf("renamed path should keep readonly: %+v", newState)
	}
}

func TestLockStoreRewriteRenamedDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &LockStore{}
	ws := t.TempDir()
	oldDir := filepath.Join(ws, "notes")
	oldPath := filepath.Join(oldDir, "child.md")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ws, oldPath, false); err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(ws, "docs")
	if err := os.Rename(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if err := store.Rewrite(ws, oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(newDir, "child.md")
	newState, err := store.Get(ws, newPath)
	if err != nil {
		t.Fatal(err)
	}
	if !newState.Found || newState.Editing {
		t.Fatalf("renamed directory should keep child lock: %+v", newState)
	}
}
