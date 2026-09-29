package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigratesLegacyDataDir(t *testing.T) {
	home := withTempHome(t)
	legacy := filepath.Join(home, legacyDataDirName)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, sessionFileName), []byte(`{"last":{"kind":"directory","path":"/tmp/ws"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir, err := dataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(home, dataDirName) {
		t.Fatalf("data dir = %s", dir)
	}
	for _, name := range []string{sessionFileName, "settings.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("legacy %s should migrate to %s: %v", name, dataDirName, err)
		}
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy %s should be removed, got %v", legacyDataDirName, err)
	}
}

func TestExistingDataDirIsNotReplacedByLegacy(t *testing.T) {
	home := withTempHome(t)
	current := filepath.Join(home, dataDirName)
	if err := os.MkdirAll(current, 0o700); err != nil {
		t.Fatal(err)
	}
	want := `{"language":"zh"}` + "\n"
	if err := os.WriteFile(filepath.Join(current, "settings.json"), []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(home, legacyDataDirName)
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte(`{"language":"en"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := dataDir(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(current, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != want {
		t.Fatalf("existing %s data must win: %s", dataDirName, raw)
	}
}

func TestLastClosedWindowOwnsTheSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	s := newAppService()
	first := &OpenTarget{Kind: "directory", Path: t.TempDir()}
	second := &OpenTarget{Kind: "directory", Path: t.TempDir()}
	s.rememberWindowTarget(1, first)
	s.rememberWindowTarget(2, second)

	s.saveSessionTarget(s.windowTarget(1))
	s.saveSessionTarget(s.windowTarget(2))
	s.saveSessionTarget(s.windowTarget(1))

	sess, err := s.session.Load()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Last == nil || filepath.ToSlash(sess.Last.Path) != filepath.ToSlash(first.Path) {
		t.Fatalf("session should describe the window closed last: %+v", sess.Last)
	}
}

func TestWindowWithoutWorkspaceKeepsTheSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	s := newAppService()
	opened := &OpenTarget{Kind: "directory", Path: t.TempDir()}
	s.rememberWindowTarget(1, opened)
	if err := s.persist(opened); err != nil {
		t.Fatal(err)
	}

	// 仍停留在启动页的窗口没有任何工作区可以交出去。
	if got := s.windowTarget(2); got != nil {
		t.Fatalf("unknown window should have no workspace: %+v", got)
	}
	s.saveSessionTarget(s.windowTarget(2))

	sess, err := s.session.Load()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Last == nil || filepath.ToSlash(sess.Last.Path) != filepath.ToSlash(opened.Path) {
		t.Fatalf("session should survive an empty window: %+v", sess.Last)
	}
}
