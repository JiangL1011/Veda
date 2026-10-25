package app

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"veda/internal/model"
)

var AssociatedFileExtensions = []string{".md", ".markdown", ".txt", ".text"}

var associatedFileExtensionSet = map[string]struct{}{
	".md":       {},
	".markdown": {},
	".txt":      {},
	".text":     {},
}

// OpenTargetForPath validates a path received from the operating system and
// converts it to the same target shape used by the in-app open dialogs.
func OpenTargetForPath(path, workingDir string) *model.OpenTarget {
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
		return &model.OpenTarget{Kind: "directory", Path: abs}
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if _, ok := associatedFileExtensionSet[strings.ToLower(filepath.Ext(abs))]; !ok {
		return nil
	}
	return &model.OpenTarget{Kind: "file", Path: abs}
}

func OpenTargetsFromArgs(args []string, workingDir string) []*model.OpenTarget {
	if len(args) < 2 {
		return nil
	}
	targets := make([]*model.OpenTarget, 0, len(args)-1)
	seen := make(map[string]struct{}, len(args)-1)
	for _, arg := range args[1:] {
		target := OpenTargetForPath(arg, workingDir)
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

// ExternalOpenCoordinator funnels LaunchServices, command-line and
// single-instance requests through one window-opening path.
type ExternalOpenCoordinator struct {
	svc *AppService

	mu               sync.Mutex
	app              *application.App
	initialWindow    application.Window
	initialClaimable bool
	suppressed       map[string]int
}

func NewExternalOpenCoordinator(svc *AppService) *ExternalOpenCoordinator {
	return &ExternalOpenCoordinator{
		svc:        svc,
		suppressed: make(map[string]int),
	}
}

func (c *ExternalOpenCoordinator) Attach(app *application.App) {
	c.mu.Lock()
	c.app = app
	c.mu.Unlock()
}

func (c *ExternalOpenCoordinator) SetInitialWindow(win application.Window, claimable bool) {
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

func (c *ExternalOpenCoordinator) SuppressFrameworkOpen(path string) {
	c.mu.Lock()
	c.suppressed[filepath.Clean(path)]++
	c.mu.Unlock()
}

func (c *ExternalOpenCoordinator) ConsumeSuppressed(path string) bool {
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

func (c *ExternalOpenCoordinator) ClaimInitialWindow() application.Window {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.initialClaimable || c.initialWindow == nil {
		return nil
	}
	c.initialClaimable = false
	return c.initialWindow
}

func (c *ExternalOpenCoordinator) currentApp() *application.App {
	c.mu.Lock()
	app := c.app
	c.mu.Unlock()
	if app != nil {
		return app
	}
	return application.Get()
}

func (c *ExternalOpenCoordinator) OpenPath(path, workingDir string, frameworkEvent bool) {
	target := OpenTargetForPath(path, workingDir)
	if target == nil {
		return
	}
	if frameworkEvent && c.ConsumeSuppressed(target.Path) {
		return
	}
	c.OpenTarget(target)
}

func (c *ExternalOpenCoordinator) OpenArgs(args []string, workingDir string, frameworkEvent bool) {
	for _, target := range OpenTargetsFromArgs(args, workingDir) {
		if frameworkEvent && c.ConsumeSuppressed(target.Path) {
			continue
		}
		c.OpenTarget(target)
	}
}

func (c *ExternalOpenCoordinator) OpenTarget(target *model.OpenTarget) {
	if target == nil || c.svc == nil {
		return
	}
	app := c.currentApp()
	if app == nil {
		return
	}
	_ = c.svc.persist(target)
	if win := c.ClaimInitialWindow(); win != nil {
		c.svc.rememberWindowTarget(win.ID(), target)
		win.SetURL(windowURL(false, target))
		win.Show()
		win.Focus()
		return
	}
	win := OpenTargetWindow(app, c.svc, target)
	if win != nil {
		win.Show()
		win.Focus()
	}
}
