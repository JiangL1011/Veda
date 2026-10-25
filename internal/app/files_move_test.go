package app

import (
	"os"
	"path/filepath"
	"testing"
	"veda/internal/model"
)

func TestMoveEntryIntoFolderKeepsIndex(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	ws := t.TempDir()

	notes, err := svc.CreateFolder(ws, "notes")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := svc.CreateFolder(ws, "archive")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.CreateDocument(notes.Path, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(doc.Path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	docID := mustIndexID(t, doc.Path)

	moved, err := svc.Move(doc.Path, archive.Path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(archive.Path, "a.md")
	if moved.Path != want || moved.Name != "a.md" || moved.IsDir {
		t.Fatalf("unexpected move result: %+v", moved)
	}
	if raw, err := os.ReadFile(want); err != nil || string(raw) != "hello" {
		t.Fatalf("moved file content: %v %q", err, raw)
	}
	if _, err := os.Stat(doc.Path); !os.IsNotExist(err) {
		t.Fatalf("source should be gone: %v", err)
	}
	if id := mustIndexID(t, want); id != docID {
		t.Fatalf("file id should follow the move: %s vs %s", id, docID)
	}
	if _, ok := mustReadFileIndex(t)[doc.Path]; ok {
		t.Fatal("old path should be removed from file-index.json")
	}
}

func TestMoveFolderKeepsNestedIndex(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	ws := t.TempDir()

	docs, err := svc.CreateFolder(ws, "docs")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := svc.CreateFolder(docs.Path, "sub")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := svc.CreateDocument(sub.Path, "c.md")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := svc.CreateFolder(ws, "archive")
	if err != nil {
		t.Fatal(err)
	}
	folderID := mustIndexID(t, docs.Path)
	nestedID := mustIndexID(t, nested.Path)

	moved, err := svc.Move(docs.Path, archive.Path)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(archive.Path, "docs"); moved.Path != want {
		t.Fatalf("moved folder path = %q, want %q", moved.Path, want)
	}
	movedNested := filepath.Join(moved.Path, "sub", "c.md")
	if _, err := os.Stat(movedNested); err != nil {
		t.Fatal(err)
	}
	if id := mustIndexID(t, moved.Path); id != folderID {
		t.Fatalf("folder id should follow the move: %s vs %s", id, folderID)
	}
	if id := mustIndexID(t, movedNested); id != nestedID {
		t.Fatalf("nested id should follow the move: %s vs %s", id, nestedID)
	}
	index := mustReadFileIndex(t)
	for _, old := range []string{docs.Path, sub.Path, nested.Path} {
		if _, ok := index[old]; ok {
			t.Fatalf("old path %s should be removed from the index", old)
		}
	}
}

func TestMoveEntryRejectsFolderIntoItself(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	ws := t.TempDir()

	folder, err := svc.CreateFolder(ws, "folder")
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.CreateFolder(folder.Path, "child")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Move(folder.Path, child.Path); err == nil {
		t.Fatal("moving a folder into its own child should fail")
	} else if code := errorCodeOf(t, err); code != errCodeMoveIntoSelf {
		t.Fatalf("got code %q, want %q", code, errCodeMoveIntoSelf)
	}
	if _, err := os.Stat(folder.Path); err != nil {
		t.Fatalf("source folder must survive a rejected move: %v", err)
	}
}

func TestMoveEntryIntoSameFolderIsNoop(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	ws := t.TempDir()

	notes, err := svc.CreateFolder(ws, "notes")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.CreateDocument(notes.Path, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	id := mustIndexID(t, doc.Path)

	same, err := svc.Move(doc.Path, notes.Path)
	if err != nil {
		t.Fatal(err)
	}
	if same.Path != doc.Path || same.Name != "a.md" {
		t.Fatalf("same-folder move should be a no-op: %+v", same)
	}
	if _, err := os.Stat(filepath.Join(notes.Path, "a(1).md")); !os.IsNotExist(err) {
		t.Fatalf("no copy should be created: %v", err)
	}
	if got := mustIndexID(t, doc.Path); got != id {
		t.Fatalf("id should not change: %s vs %s", got, id)
	}
}

func TestMoveEntryResolvesNameConflict(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	ws := t.TempDir()

	notes, err := svc.CreateFolder(ws, "notes")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := svc.CreateFolder(ws, "archive")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.CreateDocument(notes.Path, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDocument(archive.Path, "a.md"); err != nil {
		t.Fatal(err)
	}
	docID := mustIndexID(t, doc.Path)

	moved, err := svc.Move(doc.Path, archive.Path)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Name != "a(1).md" {
		t.Fatalf("conflicting name should get a suffix: %q", moved.Name)
	}
	if id := mustIndexID(t, moved.Path); id != docID {
		t.Fatalf("id should follow the renamed move: %s vs %s", id, docID)
	}
}

func TestMoveThroughServiceRewritesSession(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	ws := t.TempDir()

	notes, err := svc.CreateFolder(ws, "notes")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := svc.CreateFolder(ws, "archive")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.CreateDocument(notes.Path, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.persist(&model.OpenTarget{Kind: "directory", Path: ws, File: doc.Path}); err != nil {
		t.Fatal(err)
	}

	moved, err := svc.Move(doc.Path, archive.Path)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := svc.store.Session.Load()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Last == nil || sess.Last.File != moved.Path {
		t.Fatalf("session should follow the move: %+v", sess.Last)
	}
}
