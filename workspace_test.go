package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func mustWorkspaceDataDir(t *testing.T, ws string) string {
	t.Helper()
	dir, err := workspaceDataDir(ws)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func mustWorkspaceFileBytes(t *testing.T, ws, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(mustWorkspaceDataDir(t, ws), name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertNoWorkspaceDotDir(t *testing.T, ws string) {
	t.Helper()
	for _, name := range []string{dataDirName, legacyDataDirName} {
		if _, err := os.Stat(filepath.Join(ws, name)); err == nil {
			t.Fatalf("workspace must not contain %s", name)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}

func TestWorkspaceDataDirUsesGlobalUUID(t *testing.T) {
	home := withTempHome(t)
	ws := t.TempDir()

	first := mustWorkspaceDataDir(t, ws)
	second := mustWorkspaceDataDir(t, ws)
	if first != second {
		t.Fatalf("same workspace should reuse the store: %s vs %s", first, second)
	}
	if filepath.Dir(first) != filepath.Join(home, dataDirName) {
		t.Fatalf("store should live under global %s, got %s", dataDirName, first)
	}
	assertNoWorkspaceDotDir(t, ws)

	raw, err := os.ReadFile(filepath.Join(home, dataDirName, workspaceMapFileName))
	if err != nil {
		t.Fatal(err)
	}
	var idx workspaceIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(ws)
	if err != nil {
		t.Fatal(err)
	}
	id := idx.Workspaces[abs]
	if id == "" {
		t.Fatalf("workspace.json missing mapping: %s", raw)
	}
	if filepath.Base(first) != id {
		t.Fatalf("store dir %s should match uuid %s", first, id)
	}

	other := t.TempDir()
	otherDir := mustWorkspaceDataDir(t, other)
	if otherDir == first {
		t.Fatal("different workspaces should not share a uuid")
	}
}

func TestMigratesLegacyWorkspaceDir(t *testing.T) {
	for _, name := range []string{dataDirName, legacyDataDirName} {
		t.Run(name, func(t *testing.T) {
			withTempHome(t)
			ws := t.TempDir()
			legacy := filepath.Join(ws, name)
			if err := os.MkdirAll(legacy, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(legacy, "layout.json"), []byte(`{"sidebarWidth":321}`+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(legacy, "locks.json"), []byte(`{"files":{"a.md":"readonly"}}`+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			store := &layoutStore{}
			got, err := store.load(ws)
			if err != nil {
				t.Fatal(err)
			}
			if got.SidebarWidth != 321 {
				t.Fatalf("migrated layout: %+v", got)
			}
			assertNoWorkspaceDotDir(t, ws)
			if _, err := os.Stat(filepath.Join(mustWorkspaceDataDir(t, ws), "locks.json")); err != nil {
				t.Fatalf("legacy files should move with layout: %v", err)
			}
		})
	}
}
