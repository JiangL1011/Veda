package app

import (
	"path/filepath"
	"testing"
)

func TestFileOperationsKeepIndexInSync(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	ws := t.TempDir()

	folder, err := svc.CreateFolder(ws, "notes")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.CreateDocument(folder.Path, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	if mustIndexID(t, ws) == "" || mustIndexID(t, folder.Path) == "" {
		t.Fatal("workspace and folder should be indexed")
	}
	docID := mustIndexID(t, doc.Path)

	renamed, err := svc.Rename(doc.Path, "b.md")
	if err != nil {
		t.Fatal(err)
	}
	if id := mustIndexID(t, renamed.Path); id != docID {
		t.Fatalf("rename should keep the id: %s vs %s", id, docID)
	}
	if _, ok := mustReadFileIndex(t)[doc.Path]; ok {
		t.Fatal("old name should disappear from the index")
	}

	sub, err := svc.CreateFolder(folder.Path, "sub")
	if err != nil {
		t.Fatal(err)
	}
	nested, err := svc.CreateDocument(sub.Path, "c.md")
	if err != nil {
		t.Fatal(err)
	}
	nestedID := mustIndexID(t, nested.Path)

	moved, err := svc.Rename(folder.Path, "archive")
	if err != nil {
		t.Fatal(err)
	}
	movedNested := filepath.Join(moved.Path, "sub", "c.md")
	if id := mustIndexID(t, movedNested); id != nestedID {
		t.Fatalf("folder rename should keep child ids: %s vs %s", id, nestedID)
	}
	index := mustReadFileIndex(t)
	if _, ok := index[folder.Path]; ok {
		t.Fatal("old folder path should be removed")
	}

	if err := svc.Delete(moved.Path); err != nil {
		t.Fatal(err)
	}
	index = mustReadFileIndex(t)
	for _, gone := range []string{moved.Path, movedNested} {
		if _, ok := index[gone]; ok {
			t.Fatalf("deleted path %s should be removed from the index", gone)
		}
	}
}

func TestTrashKeepsFileIndex(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	ws := t.TempDir()

	folder, err := svc.CreateFolder(ws, "notes")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := svc.CreateDocument(folder.Path, "a.md")
	if err != nil {
		t.Fatal(err)
	}
	folderID := mustIndexID(t, folder.Path)
	docID := mustIndexID(t, doc.Path)

	// 模拟移入回收站：文件可能之后被还原到原路径，索引必须保留。
	noopTrash := func(string) error { return nil }
	if err := svc.removeEntry(folder.Path, noopTrash, true); err != nil {
		t.Fatal(err)
	}
	index := mustReadFileIndex(t)
	if index[folder.Path] != folderID || index[doc.Path] != docID {
		t.Fatalf("trashing must keep file ids: %v", index)
	}

	// 永久删除才清理索引。
	if err := svc.removeEntry(folder.Path, noopTrash, false); err != nil {
		t.Fatal(err)
	}
	index = mustReadFileIndex(t)
	if _, ok := index[folder.Path]; ok {
		t.Fatal("permanent delete should remove the folder from the index")
	}
	if _, ok := index[doc.Path]; ok {
		t.Fatal("permanent delete should remove nested files from the index")
	}
}
