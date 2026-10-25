package model

// WindowLayout 是某个工作区最后一次记录的窗口几何信息。
type WindowLayout struct {
	ScreenID   string `json:"screenID,omitempty"`
	ScreenName string `json:"screenName,omitempty"`
	X          int    `json:"x"`
	Y          int    `json:"y"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Maximized  bool   `json:"maximized,omitempty"`
	Fullscreen bool   `json:"fullscreen,omitempty"`
}

// WorkspaceLayout 持久化在 ~/.veda/{workspace-uuid}/layout.json 中。
type WorkspaceLayout struct {
	SidebarWidth      int           `json:"sidebarWidth,omitempty"`
	HeadingPanelWidth int           `json:"headingPanelWidth,omitempty"`
	OpenTabs          []string      `json:"openTabs,omitempty"`
	ActiveTab         string        `json:"activeTab,omitempty"`
	Window            *WindowLayout `json:"window,omitempty"`
}
