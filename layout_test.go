package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeSidebarWidth(t *testing.T) {
	if got := normalizeSidebarWidth(0); got != defaultSidebarWidth {
		t.Fatalf("empty width should default, got %d", got)
	}
	if got := normalizeSidebarWidth(120); got != minSidebarWidth {
		t.Fatalf("min clamp: %d", got)
	}
	if got := normalizeSidebarWidth(900); got != maxSidebarWidth {
		t.Fatalf("max clamp: %d", got)
	}
	if got := normalizeSidebarWidth(300); got != 300 {
		t.Fatalf("in-range width: %d", got)
	}
}

func TestNormalizeHeadingPanelWidth(t *testing.T) {
	if got := normalizeHeadingPanelWidth(0); got != defaultHeadingPanelWidth {
		t.Fatalf("empty width should default, got %d", got)
	}
	if got := normalizeHeadingPanelWidth(80); got != minHeadingPanelWidth {
		t.Fatalf("min clamp: %d", got)
	}
	if got := normalizeHeadingPanelWidth(900); got != maxHeadingPanelWidth {
		t.Fatalf("max clamp: %d", got)
	}
	if got := normalizeHeadingPanelWidth(280); got != 280 {
		t.Fatalf("in-range width: %d", got)
	}
}

func TestLayoutStoreSidebarAndWindowMerge(t *testing.T) {
	withTempHome(t)
	ws := t.TempDir()
	store := &layoutStore{}

	loaded, err := store.load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SidebarWidth != defaultSidebarWidth || loaded.HeadingPanelWidth != defaultHeadingPanelWidth || loaded.Window != nil {
		t.Fatalf("empty layout: %+v", loaded)
	}

	if _, err := store.saveSidebarWidth(ws, 320); err != nil {
		t.Fatal(err)
	}
	if _, err := store.saveHeadingPanelWidth(ws, 280); err != nil {
		t.Fatal(err)
	}
	if err := store.saveWindow(ws, &WindowLayout{
		ScreenID:   "display-2",
		ScreenName: "DELL",
		X:          40,
		Y:          60,
		Width:      1400,
		Height:     900,
		Maximized:  true,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarWidth != 320 {
		t.Fatalf("sidebar: %d", got.SidebarWidth)
	}
	if got.HeadingPanelWidth != 280 {
		t.Fatalf("heading panel: %d", got.HeadingPanelWidth)
	}
	if got.Window == nil || got.Window.ScreenID != "display-2" || got.Window.Width != 1400 || !got.Window.Maximized {
		t.Fatalf("window: %+v", got.Window)
	}

	if _, err := store.saveSidebarWidth(ws, 200); err != nil {
		t.Fatal(err)
	}
	got, err = store.load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarWidth != 200 {
		t.Fatalf("updated sidebar: %d", got.SidebarWidth)
	}
	if got.HeadingPanelWidth != 280 {
		t.Fatalf("heading panel should survive sidebar save: %d", got.HeadingPanelWidth)
	}
	if got.Window == nil || got.Window.Height != 900 {
		t.Fatalf("window should survive sidebar save: %+v", got.Window)
	}

	raw := mustWorkspaceFileBytes(t, ws, "layout.json")
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["sidebarWidth"] != float64(200) {
		t.Fatalf("json sidebar: %s", raw)
	}
	if doc["headingPanelWidth"] != float64(280) {
		t.Fatalf("json heading panel: %s", raw)
	}
	assertNoWorkspaceDotDir(t, ws)
}

func TestLayoutStoreRejectsFileWorkspace(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.md")
	if err := os.WriteFile(file, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &layoutStore{}
	if _, err := store.saveSidebarWidth(file, 240); err == nil {
		t.Fatal("expected error for file workspace")
	}
	if _, err := store.saveHeadingPanelWidth(file, 240); err == nil {
		t.Fatal("expected error for file workspace heading panel")
	}
}

func TestAppServiceSidebarLayoutAPI(t *testing.T) {
	withTempHome(t)
	ws := t.TempDir()
	svc := newAppService()
	got, err := svc.GetLayout(ws)
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarWidth != defaultSidebarWidth {
		t.Fatalf("default sidebar: %d", got.SidebarWidth)
	}
	if got.HeadingPanelWidth != defaultHeadingPanelWidth {
		t.Fatalf("default heading panel: %d", got.HeadingPanelWidth)
	}
	saved, err := svc.SaveSidebarWidth(ws, 360)
	if err != nil {
		t.Fatal(err)
	}
	if saved.SidebarWidth != 360 {
		t.Fatalf("saved: %d", saved.SidebarWidth)
	}
	heading, err := svc.SaveHeadingPanelWidth(ws, 300)
	if err != nil {
		t.Fatal(err)
	}
	if heading.HeadingPanelWidth != 300 {
		t.Fatalf("saved heading panel: %d", heading.HeadingPanelWidth)
	}
	again, err := svc.GetLayout(ws)
	if err != nil {
		t.Fatal(err)
	}
	if again.SidebarWidth != 360 {
		t.Fatalf("reload: %d", again.SidebarWidth)
	}
	if again.HeadingPanelWidth != 300 {
		t.Fatalf("reload heading panel: %d", again.HeadingPanelWidth)
	}
}

func TestLayoutStoreOpenTabsSkipMissingFiles(t *testing.T) {
	withTempHome(t)
	ws := t.TempDir()
	keep := filepath.Join(ws, "keep.md")
	gone := filepath.Join(ws, "gone.md")
	if err := os.WriteFile(keep, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	keep, err := filepath.Abs(keep)
	if err != nil {
		t.Fatal(err)
	}
	store := &layoutStore{}
	saved, err := store.saveOpenTabs(ws, []string{keep, gone, keep}, gone)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.OpenTabs) != 1 || saved.OpenTabs[0] != keep {
		t.Fatalf("missing tabs should be dropped: %+v", saved.OpenTabs)
	}
	if saved.ActiveTab != "" {
		t.Fatalf("missing active tab should be cleared, got %s", saved.ActiveTab)
	}

	again, err := store.saveOpenTabs(ws, []string{keep, gone}, keep)
	if err != nil {
		t.Fatal(err)
	}
	if again.ActiveTab != keep {
		t.Fatalf("active: %s", again.ActiveTab)
	}
	raw := mustWorkspaceFileBytes(t, ws, "layout.json")
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	tabs, _ := doc["openTabs"].([]any)
	if len(tabs) != 1 || tabs[0] != keep {
		t.Fatalf("json tabs: %s", raw)
	}
	if doc["activeTab"] != keep {
		t.Fatalf("json active: %s", raw)
	}
}

func TestLayoutStoreIgnoresBrokenWindow(t *testing.T) {
	withTempHome(t)
	ws := t.TempDir()
	path, err := workspaceLayoutPath(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"sidebarWidth":220,"headingPanelWidth":80,"window":{"x":10,"y":10,"width":0,"height":0}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &layoutStore{}
	got, err := store.load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if got.SidebarWidth != 220 {
		t.Fatalf("sidebar: %d", got.SidebarWidth)
	}
	if got.HeadingPanelWidth != minHeadingPanelWidth {
		t.Fatalf("heading panel should clamp stored min: %d", got.HeadingPanelWidth)
	}
	if got.Window != nil {
		t.Fatalf("invalid window should be dropped: %+v", got.Window)
	}
}
