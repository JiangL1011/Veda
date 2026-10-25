package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileIndexAssignsStableIDs(t *testing.T) {
	withTempHome(t)
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}

	first, err := EnsureFileID(file)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EnsureFileID(file)
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
	if err := MoveFileIndex(file, moved); err != nil {
		t.Fatal(err)
	}
	if id := mustIndexID(t, moved); id != first {
		t.Fatalf("rename should keep the id: %s vs %s", id, first)
	}
	if _, ok := mustReadFileIndex(t)[file]; ok {
		t.Fatal("old path should be removed from the index")
	}

	if err := RemoveFileIndex(moved); err != nil {
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
