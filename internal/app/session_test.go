package app

import (
	"path/filepath"
	"testing"

	"veda/internal/model"
)

func TestLastClosedWindowOwnsTheSession(t *testing.T) {
	withTempHome(t)

	s := newTestService(t)
	first := &model.OpenTarget{Kind: "directory", Path: t.TempDir()}
	second := &model.OpenTarget{Kind: "directory", Path: t.TempDir()}
	s.rememberWindowTarget(1, first)
	s.rememberWindowTarget(2, second)

	s.saveSessionTarget(s.windowTarget(1))
	s.saveSessionTarget(s.windowTarget(2))
	s.saveSessionTarget(s.windowTarget(1))

	sess, err := s.store.Session.Load()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Last == nil || filepath.ToSlash(sess.Last.Path) != filepath.ToSlash(first.Path) {
		t.Fatalf("session should describe the window closed last: %+v", sess.Last)
	}
}

func TestWindowWithoutWorkspaceKeepsTheSession(t *testing.T) {
	withTempHome(t)

	s := newTestService(t)
	opened := &model.OpenTarget{Kind: "directory", Path: t.TempDir()}
	s.rememberWindowTarget(1, opened)
	if err := s.persist(opened); err != nil {
		t.Fatal(err)
	}

	// 仍停留在启动页的窗口没有任何工作区可以交出去。
	if got := s.windowTarget(2); got != nil {
		t.Fatalf("unknown window should have no workspace: %+v", got)
	}
	s.saveSessionTarget(s.windowTarget(2))

	sess, err := s.store.Session.Load()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Last == nil || filepath.ToSlash(sess.Last.Path) != filepath.ToSlash(opened.Path) {
		t.Fatalf("session should survive an empty window: %+v", sess.Last)
	}
}
