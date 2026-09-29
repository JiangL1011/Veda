package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	layoutFileName = "layout.json"

	defaultSidebarWidth      = 256
	minSidebarWidth          = 180
	maxSidebarWidth          = 480
	defaultHeadingPanelWidth = 224
	minHeadingPanelWidth     = 160
	maxHeadingPanelWidth     = 560
)

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

type layoutStore struct {
	mu sync.Mutex
}

func workspaceLayoutPath(workspacePath string) (string, error) {
	return workspaceFile(workspacePath, layoutFileName)
}

func clampSidebarWidth(width int) int {
	if width < minSidebarWidth {
		return minSidebarWidth
	}
	if width > maxSidebarWidth {
		return maxSidebarWidth
	}
	return width
}

func normalizeSidebarWidth(width int) int {
	if width <= 0 {
		return defaultSidebarWidth
	}
	return clampSidebarWidth(width)
}

func clampHeadingPanelWidth(width int) int {
	if width < minHeadingPanelWidth {
		return minHeadingPanelWidth
	}
	if width > maxHeadingPanelWidth {
		return maxHeadingPanelWidth
	}
	return width
}

func normalizeHeadingPanelWidth(width int) int {
	if width <= 0 {
		return defaultHeadingPanelWidth
	}
	return clampHeadingPanelWidth(width)
}

func (s *layoutStore) load(workspacePath string) (*WorkspaceLayout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked(workspacePath)
}

func (s *layoutStore) loadLocked(workspacePath string) (*WorkspaceLayout, error) {
	out := &WorkspaceLayout{
		SidebarWidth:      defaultSidebarWidth,
		HeadingPanelWidth: defaultHeadingPanelWidth,
	}
	if workspacePath == "" {
		return out, nil
	}
	path, err := workspaceLayoutPath(workspacePath)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	if len(bytesTrimSpace(raw)) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &WorkspaceLayout{
			SidebarWidth:      defaultSidebarWidth,
			HeadingPanelWidth: defaultHeadingPanelWidth,
		}, nil
	}
	out.SidebarWidth = normalizeSidebarWidth(out.SidebarWidth)
	out.HeadingPanelWidth = normalizeHeadingPanelWidth(out.HeadingPanelWidth)
	out.OpenTabs, out.ActiveTab = livingOpenTabs(out.OpenTabs, out.ActiveTab)
	if out.Window != nil && (out.Window.Width <= 0 || out.Window.Height <= 0) {
		out.Window = nil
	}
	return out, nil
}

func livingOpenTabs(paths []string, active string) ([]string, string) {
	order := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		abs, ok := existingTabFile(path)
		if !ok {
			continue
		}
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		order = append(order, abs)
	}
	if abs, ok := existingTabFile(active); ok {
		if _, dup := seen[abs]; !dup {
			order = append(order, abs)
		}
		return order, abs
	}
	return order, ""
}

func existingTabFile(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return "", false
	}
	return abs, true
}

func (s *layoutStore) saveSidebarWidth(workspacePath string, width int) (*WorkspaceLayout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if workspacePath == "" {
		return nil, errors.New("当前窗口不是工作区")
	}
	current, err := s.loadLocked(workspacePath)
	if err != nil {
		return nil, err
	}
	current.SidebarWidth = clampSidebarWidth(width)
	if err := s.writeLocked(workspacePath, current); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *layoutStore) saveHeadingPanelWidth(workspacePath string, width int) (*WorkspaceLayout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if workspacePath == "" {
		return nil, errors.New("当前窗口不是工作区")
	}
	current, err := s.loadLocked(workspacePath)
	if err != nil {
		return nil, err
	}
	current.HeadingPanelWidth = clampHeadingPanelWidth(width)
	if err := s.writeLocked(workspacePath, current); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *layoutStore) saveOpenTabs(workspacePath string, tabs []string, active string) (*WorkspaceLayout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if workspacePath == "" {
		return nil, errors.New("当前窗口不是工作区")
	}
	current, err := s.loadLocked(workspacePath)
	if err != nil {
		return nil, err
	}
	current.OpenTabs, current.ActiveTab = livingOpenTabs(tabs, active)
	if err := s.writeLocked(workspacePath, current); err != nil {
		return nil, err
	}
	return current, nil
}

func (s *layoutStore) saveWindow(workspacePath string, window *WindowLayout) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if workspacePath == "" {
		return nil
	}
	if window == nil || window.Width <= 0 || window.Height <= 0 {
		return nil
	}
	current, err := s.loadLocked(workspacePath)
	if err != nil {
		return err
	}
	current.Window = window
	return s.writeLocked(workspacePath, current)
}

func (s *layoutStore) writeLocked(workspacePath string, layout *WorkspaceLayout) error {
	path, err := workspaceLayoutPath(workspacePath)
	if err != nil {
		return err
	}
	return writeJSONFile(path, layout)
}
