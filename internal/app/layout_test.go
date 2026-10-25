package app

import "testing"

func TestAppServiceSidebarLayoutAPI(t *testing.T) {
	withTempHome(t)
	ws := t.TempDir()
	svc := newTestService(t)
	got, err := svc.GetLayout(ws)
	if err != nil {
		t.Fatal(err)
	}
	// 与 store 里的默认值保持一致：侧边栏 256，大纲面板 224。
	if got.SidebarWidth != 256 {
		t.Fatalf("default sidebar: %d", got.SidebarWidth)
	}
	if got.HeadingPanelWidth != 224 {
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
