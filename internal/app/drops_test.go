package app

import (
	"os"
	"path/filepath"
	"testing"

	"veda/internal/model"
	"veda/internal/store"
)

func TestResolveDropDir(t *testing.T) {
	svc := &AppService{}
	root := t.TempDir()
	sub := filepath.Join(root, "notes")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()

	if got := svc.resolveDropDir(root, ""); got != root {
		t.Fatalf("empty hint should use the workspace root, got %s", got)
	}
	if got := svc.resolveDropDir(root, sub); got != sub {
		t.Fatalf("inside hint should win, got %s", got)
	}
	if got := svc.resolveDropDir(root, outside); got != root {
		t.Fatalf("outside hint should fall back, got %s", got)
	}
	if got := svc.resolveDropDir(root, filepath.Join(root, "missing")); got != root {
		t.Fatalf("missing hint should fall back, got %s", got)
	}
}

func TestDragDropModeFallsBackToGlobal(t *testing.T) {
	withTempHome(t)

	svc := &AppService{store: store.New()}

	global := store.DefaultSettings()
	global.General.DragDropMode = model.DragDropMove
	if _, err := svc.store.Settings.Save("global", "", global); err != nil {
		t.Fatal(err)
	}

	// 没有工作区覆盖层时沿用全局设置。
	plain := t.TempDir()
	if mode := svc.dragDropMode(plain); mode != model.DragDropMove {
		t.Fatalf("workspace without its own settings should follow global, got %s", mode)
	}

	// 工作区把自己的设置改成复制后，只影响这个工作区。
	ws := t.TempDir()
	scoped := store.DefaultSettings()
	scoped.General.DragDropMode = model.DragDropCopy
	if _, err := svc.store.Settings.Save("workspace", ws, scoped); err != nil {
		t.Fatal(err)
	}
	if mode := svc.dragDropMode(ws); mode != model.DragDropCopy {
		t.Fatalf("workspace override should win, got %s", mode)
	}
	if mode := svc.dragDropMode(plain); mode != model.DragDropMove {
		t.Fatalf("other workspaces should stay on global, got %s", mode)
	}
}

func TestApplyFileDropRecordsOutcome(t *testing.T) {
	withTempHome(t)
	svc := newTestService(t)
	root := t.TempDir()
	src := t.TempDir()
	from := filepath.Join(src, "note.md")
	writeFile(t, from, "note")

	outcome := svc.applyFileDrop(nil, []string{from}, root, root, false)
	if len(outcome.Errors) != 0 || len(outcome.Imported) != 1 {
		t.Fatalf("got %+v", outcome)
	}
	if !svc.isAllowed(outcome.Imported[0].Path) {
		t.Fatal("imported file should be allowed for later reads")
	}
	copiedID := mustIndexID(t, outcome.Imported[0].Path)

	// 移动之后源文件不再需要单独放行，索引编号跟着文件走到新位置。
	sourceID, err := store.EnsureFileID(from)
	if err != nil {
		t.Fatal(err)
	}
	moved := svc.applyFileDrop(nil, []string{from}, root, root, true)
	if len(moved.Errors) != 0 || moved.Imported[0].Name != "note(1).md" {
		t.Fatalf("move: %+v", moved)
	}
	if svc.isAllowed(from) {
		t.Fatal("moved source should no longer be allowed")
	}
	if id := mustIndexID(t, moved.Imported[0].Path); id != sourceID {
		t.Fatalf("move should keep the id: %s vs %s", id, sourceID)
	}
	if copiedID == sourceID {
		t.Fatal("copied file should get its own id")
	}
	if _, ok := mustReadFileIndex(t)[from]; ok {
		t.Fatal("moved source should be removed from the index")
	}
}

func TestResolveFileDropCancel(t *testing.T) {
	svc := &AppService{pendingDrops: map[uint]*pendingFileDrop{}}
	svc.rememberPendingDrop(7, &pendingFileDrop{files: []string{"/tmp/a"}, destDir: "/tmp", workspace: "/tmp"})
	if drop := svc.takePendingDrop(7); drop == nil {
		t.Fatal("pending drop should be recorded")
	}
	if drop := svc.takePendingDrop(7); drop != nil {
		t.Fatal("pending drop should be consumed once")
	}
}
