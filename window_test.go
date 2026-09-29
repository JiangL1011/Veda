package main

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestClampWindowSize(t *testing.T) {
	w, h := clampWindowSize(100, 100)
	if w != minWindowWidth || h != minWindowHeight {
		t.Fatalf("min size: %dx%d", w, h)
	}
	w, h = clampWindowSize(1600, 1000)
	if w != 1600 || h != 1000 {
		t.Fatalf("passthrough: %dx%d", w, h)
	}
}

func TestMatchSavedScreenFallsBackWhenMissing(t *testing.T) {
	primary := &application.Screen{ID: "built-in", Name: "Built-in"}
	external := &application.Screen{ID: "dell", Name: "DELL"}
	screens := []*application.Screen{primary, external}

	if got := matchSavedScreen(screens, &WindowLayout{ScreenID: "dell", ScreenName: "DELL"}); got != external {
		t.Fatalf("should keep the recorded display, got %+v", got)
	}
	if got := matchSavedScreen(screens, &WindowLayout{ScreenID: "gone", ScreenName: "DELL"}); got != nil {
		t.Fatalf("missing id must not match by name: %+v", got)
	}
	if got := matchSavedScreen([]*application.Screen{primary}, &WindowLayout{ScreenID: "dell", ScreenName: "DELL"}); got != nil {
		t.Fatalf("unplugged display should be treated as missing: %+v", got)
	}
	if got := matchSavedScreen(screens, &WindowLayout{ScreenName: "Built-in"}); got != primary {
		t.Fatalf("name-only layout should match: %+v", got)
	}
}

func TestClampLayoutToScreen(t *testing.T) {
	layout := &WindowLayout{X: -20, Y: -8, Width: 80, Height: 40}
	clampLayoutToScreen(layout, nil)
	if layout.X != 0 || layout.Y != 0 || layout.Width != minWindowWidth || layout.Height != minWindowHeight {
		t.Fatalf("nil screen: %+v", layout)
	}
}

func TestSpawnWindowLayoutIsThreeQuartersScreenAndOffsets(t *testing.T) {
	work := application.Rect{Width: 3000, Height: 2000}
	parent := WindowLayout{X: 800, Y: 600, Width: 1200, Height: 800, ScreenID: "built-in"}

	got := spawnWindowLayout(parent, work)
	if got.Width != 2250 || got.Height != 1500 {
		t.Fatalf("should be three quarters of the work area: %+v", got)
	}
	if got.Maximized || got.Fullscreen {
		t.Fatalf("spawned window must open normal: %+v", got)
	}
	if got.ScreenID != parent.ScreenID {
		t.Fatalf("should stay on the parent display: %+v", got)
	}
	parentCenterX, parentCenterY := parent.X+parent.Width/2, parent.Y+parent.Height/2
	if got.X+got.Width/2 != parentCenterX+spawnWindowCenterStep {
		t.Fatalf("centre should shift right by one step: %+v", got)
	}
	if got.Y+got.Height/2 != parentCenterY+spawnWindowCenterStep {
		t.Fatalf("centre should shift down by one step: %+v", got)
	}
}

func TestSpawnWindowLayoutIgnoresParentSize(t *testing.T) {
	work := application.Rect{Width: 1600, Height: 1200}
	parent := WindowLayout{X: 0, Y: 0, Width: 900, Height: 600}

	got := spawnWindowLayout(parent, work)
	if got.Width != 1200 || got.Height != 900 {
		t.Fatalf("three quarters of the work area regardless of parent: %+v", got)
	}
	if got.X < 0 || got.Y < 0 || got.X+got.Width > work.Width || got.Y+got.Height > work.Height {
		t.Fatalf("should stay inside the work area: %+v", got)
	}
}

func TestSpawnWindowLayoutKeepsMinimumSizeOnSmallScreens(t *testing.T) {
	work := application.Rect{Width: 900, Height: 600}
	parent := WindowLayout{X: 20, Y: 20, Width: 860, Height: 560}

	got := spawnWindowLayout(parent, work)
	if got.Width != minWindowWidth || got.Height != minWindowHeight {
		t.Fatalf("three quarters of a small screen is below the minimum: %+v", got)
	}
	if got.X+got.Width > work.Width || got.Y+got.Height > work.Height {
		t.Fatalf("should stay inside the work area: %+v", got)
	}
}

func TestDefaultWindowOptionsOptInToApplicationMenu(t *testing.T) {
	if !defaultWindowOptions("/").UseApplicationMenu {
		t.Fatal("Windows and Linux windows would open without a menu bar")
	}
}

func TestWindowURLCarriesTarget(t *testing.T) {
	if got := windowURL(false, nil); got != "/" {
		t.Fatalf("plain window: %q", got)
	}
	if got := windowURL(true, nil); got != "/?restore=1" {
		t.Fatalf("restoring window: %q", got)
	}
	raw := windowURL(true, &OpenTarget{Kind: "directory", Path: "/tmp/我的 笔记"})
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	var target OpenTarget
	if err := json.Unmarshal([]byte(parsed.Query().Get("open")), &target); err != nil {
		t.Fatalf("decode target: %v", err)
	}
	if target.Kind != "directory" || target.Path != "/tmp/我的 笔记" {
		t.Fatalf("round trip: %+v", target)
	}
}
