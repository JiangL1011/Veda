package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func mustReadFileIndex(t *testing.T) map[string]string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, dataDirName, fileIndexFileName))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	if err := json.Unmarshal(raw, &files); err != nil {
		t.Fatalf("file-index.json should be a path → id object: %v\n%s", err, raw)
	}
	return files
}

func mustIndexID(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	id := mustReadFileIndex(t)[abs]
	if !isUUID(id) {
		t.Fatalf("%s should have a uuid in file-index.json, got %q", abs, id)
	}
	return id
}

func TestFileIndexAssignsStableIDs(t *testing.T) {
	withTempHome(t)
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := ensureFileID(file)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ensureFileID(file)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !isUUID(first) {
		t.Fatalf("ids should be stable uuids: %q %q", first, second)
	}
	if id := mustIndexID(t, file); id != first {
		t.Fatalf("index id mismatch: %s vs %s", id, first)
	}
	if path, ok := pathForFileID(first); !ok || path != file {
		t.Fatalf("pathForFileID: %q %v", path, ok)
	}

	moved := filepath.Join(filepath.Dir(file), "renamed.md")
	if err := os.Rename(file, moved); err != nil {
		t.Fatal(err)
	}
	if err := moveFileIndex(file, moved); err != nil {
		t.Fatal(err)
	}
	if id := mustIndexID(t, moved); id != first {
		t.Fatalf("rename should keep the id: %s vs %s", id, first)
	}
	if _, ok := mustReadFileIndex(t)[file]; ok {
		t.Fatal("old path should be removed from the index")
	}

	if err := removeFileIndex(moved); err != nil {
		t.Fatal(err)
	}
	if _, ok := mustReadFileIndex(t)[moved]; ok {
		t.Fatal("deleted path should be removed from the index")
	}
}

func TestMigrateFileIndexRewritesExistingConfigs(t *testing.T) {
	home := withTempHome(t)
	ws := t.TempDir()
	ws, err := filepath.Abs(ws)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(ws, "notes", "hello.md")
	writeFile(t, file, "hi")

	wsID := "11111111-1111-4111-8111-111111111111"
	veda := filepath.Join(home, dataDirName)
	workspaceDir := filepath.Join(veda, wsID)
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(veda, workspaceMapFileName), &workspaceIndex{
		Workspaces: map[string]string{ws: wsID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(veda, sessionFileName), map[string]any{
		"last": map[string]any{"kind": "directory", "path": ws, "file": file},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(workspaceDir, layoutFileName), map[string]any{
		"openTabs":  []any{file},
		"activeTab": file,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(workspaceDir, locksFileName), map[string]any{
		"files": map[string]any{"notes/hello.md": lockModeReadonly},
	}); err != nil {
		t.Fatal(err)
	}

	if err := migrateFileIndex(nil); err != nil {
		t.Fatal(err)
	}

	index := mustReadFileIndex(t)
	fileID := index[file]
	if !isUUID(fileID) {
		t.Fatalf("file should be indexed: %v", index)
	}
	if !isUUID(index[ws]) {
		t.Fatalf("workspace directory should be indexed: %v", index)
	}

	layout := readJSONMap(t, filepath.Join(workspaceDir, layoutFileName))
	tabs, _ := layout["openTabs"].([]any)
	if len(tabs) != 1 || tabs[0] != fileID {
		t.Fatalf("layout tabs should store ids: %v", layout)
	}
	if layout["activeTab"] != fileID {
		t.Fatalf("layout active tab should store id: %v", layout)
	}

	locks := readJSONMap(t, filepath.Join(workspaceDir, locksFileName))
	lockFiles, _ := locks["files"].(map[string]any)
	if lockFiles[fileID] != lockModeReadonly {
		t.Fatalf("locks should be keyed by id: %v", locks)
	}

	session := readJSONMap(t, filepath.Join(veda, sessionFileName))
	last, _ := session["last"].(map[string]any)
	if last["path"] != wsID {
		t.Fatalf("session workspace should store the workspace id: %v", session)
	}
	if last["file"] != fileID {
		t.Fatalf("session file should store the file id: %v", session)
	}

	// 读取侧应能把编号还原成原路径。
	if got := storedFilePath(fileID); got != file {
		t.Fatalf("storedFilePath(%s) = %q, want %q", fileID, got, file)
	}

	// 迁移可以安全重跑：已经写进配置里的编号不能被重新分配。
	if err := migrateFileIndex(nil); err != nil {
		t.Fatal(err)
	}
	if again := mustReadFileIndex(t)[file]; again != fileID {
		t.Fatalf("re-running migration should preserve ids: %s vs %s", again, fileID)
	}
}

func TestFileOperationsKeepIndexInSync(t *testing.T) {
	withTempHome(t)
	svc := newAppService()
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
	svc := newAppService()
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

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, raw)
	}
	return out
}
