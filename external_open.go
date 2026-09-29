package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var associatedFileExtensions = []string{".md", ".markdown", ".txt", ".text"}

var associatedFileExtensionSet = map[string]struct{}{
	".md":       {},
	".markdown": {},
	".txt":      {},
	".text":     {},
}

// openTargetForPath validates a path received from the operating system and
// converts it to the same target shape used by the in-app open dialogs.
func openTargetForPath(path, workingDir string) *OpenTarget {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) && workingDir != "" {
		path = filepath.Join(workingDir, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil
	}
	if info.IsDir() {
		return &OpenTarget{Kind: "directory", Path: abs}
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if _, ok := associatedFileExtensionSet[strings.ToLower(filepath.Ext(abs))]; !ok {
		return nil
	}
	return &OpenTarget{Kind: "file", Path: abs}
}

func openTargetsFromArgs(args []string, workingDir string) []*OpenTarget {
	if len(args) < 2 {
		return nil
	}
	targets := make([]*OpenTarget, 0, len(args)-1)
	seen := make(map[string]struct{}, len(args)-1)
	for _, arg := range args[1:] {
		target := openTargetForPath(arg, workingDir)
		if target == nil {
			continue
		}
		if _, ok := seen[target.Path]; ok {
			continue
		}
		seen[target.Path] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

// externalOpenCoordinator funnels LaunchServices, command-line and
// single-instance requests through one window-opening path.
type externalOpenCoordinator struct {
	svc *AppService

	mu               sync.Mutex
	app              *application.App
	initialWindow    application.Window
	initialClaimable bool
	suppressed       map[string]int
}

func newExternalOpenCoordinator(svc *AppService) *externalOpenCoordinator {
	return &externalOpenCoordinator{
		svc:        svc,
		suppressed: make(map[string]int),
	}
}

func (c *externalOpenCoordinator) attach(app *application.App) {
	c.mu.Lock()
	c.app = app
	c.mu.Unlock()
}

func (c *externalOpenCoordinator) setInitialWindow(win application.Window, claimable bool) {
	c.mu.Lock()
	c.initialWindow = win
	c.initialClaimable = claimable
	c.mu.Unlock()
	if win != nil && claimable {
		win.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
			c.mu.Lock()
			if c.initialWindow == win {
				c.initialClaimable = false
			}
			c.mu.Unlock()
		})
	}
}

func (c *externalOpenCoordinator) suppressFrameworkOpen(path string) {
	c.mu.Lock()
	c.suppressed[filepath.Clean(path)]++
	c.mu.Unlock()
}

func (c *externalOpenCoordinator) consumeSuppressed(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	path = filepath.Clean(path)
	if c.suppressed[path] == 0 {
		return false
	}
	c.suppressed[path]--
	if c.suppressed[path] == 0 {
		delete(c.suppressed, path)
	}
	return true
}

func (c *externalOpenCoordinator) claimInitialWindow() application.Window {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.initialClaimable || c.initialWindow == nil {
		return nil
	}
	c.initialClaimable = false
	return c.initialWindow
}

func (c *externalOpenCoordinator) currentApp() *application.App {
	c.mu.Lock()
	app := c.app
	c.mu.Unlock()
	if app != nil {
		return app
	}
	return application.Get()
}

func (c *externalOpenCoordinator) openPath(path, workingDir string, frameworkEvent bool) {
	target := openTargetForPath(path, workingDir)
	if target == nil {
		return
	}
	if frameworkEvent && c.consumeSuppressed(target.Path) {
		return
	}
	c.openTarget(target)
}

func (c *externalOpenCoordinator) openArgs(args []string, workingDir string, frameworkEvent bool) {
	for _, target := range openTargetsFromArgs(args, workingDir) {
		if frameworkEvent && c.consumeSuppressed(target.Path) {
			continue
		}
		c.openTarget(target)
	}
}

func (c *externalOpenCoordinator) openTarget(target *OpenTarget) {
	if target == nil || c.svc == nil {
		return
	}
	app := c.currentApp()
	if app == nil {
		return
	}
	_ = c.svc.persist(target)
	if win := c.claimInitialWindow(); win != nil {
		c.svc.rememberWindowTarget(win.ID(), target)
		win.SetURL(windowURL(false, target))
		win.Show()
		win.Focus()
		return
	}
	win := openTargetWindow(app, c.svc, target)
	if win != nil {
		win.Show()
		win.Focus()
	}
}
