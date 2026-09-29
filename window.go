package main

import (
	"encoding/json"
	"net/url"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	defaultWindowWidth  = 1200
	defaultWindowHeight = 800
	minWindowWidth      = 720
	minWindowHeight     = 480

	// 从别的窗口打开的新窗口，尺寸为工作区域的四分之三，并相对父窗口
	// 向右下方偏移一步居中。
	spawnWindowScreenPart = 0.75
	spawnWindowCenterStep = 32
)

func windowURL(restore bool, target *OpenTarget) string {
	if target != nil {
		if payload, err := json.Marshal(target); err == nil {
			return "/?" + url.Values{"open": {string(payload)}}.Encode()
		}
	}
	if restore {
		return "/?restore=1"
	}
	return "/"
}

func defaultWindowOptions(startURL string) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title:            "Veda",
		Width:            defaultWindowWidth,
		Height:           defaultWindowHeight,
		MinWidth:         minWindowWidth,
		MinHeight:        minWindowHeight,
		BackgroundColour: application.NewRGB(250, 250, 249),
		URL:              startURL,
		// Windows 和 Linux 把菜单画在每个窗口内部，所以每个窗口都必须显式
		// 启用应用菜单，否则打开后就没有菜单栏。
		UseApplicationMenu: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 40,
			Backdrop:                application.MacBackdropNormal,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	}
}

func clampWindowSize(width, height int) (int, int) {
	if width < minWindowWidth {
		width = minWindowWidth
	}
	if height < minWindowHeight {
		height = minWindowHeight
	}
	return width, height
}

func matchSavedScreen(screens []*application.Screen, layout *WindowLayout) *application.Screen {
	if layout == nil || len(screens) == 0 {
		return nil
	}
	if layout.ScreenID != "" {
		for _, screen := range screens {
			if screen != nil && screen.ID == layout.ScreenID {
				return screen
			}
		}
		// 记录的显示器已经不存在了，不要再按名称去猜。
		return nil
	}
	if layout.ScreenName != "" {
		for _, screen := range screens {
			if screen != nil && screen.Name == layout.ScreenName {
				return screen
			}
		}
	}
	return nil
}

func findScreen(app *application.App, layout *WindowLayout) *application.Screen {
	if app == nil {
		return nil
	}
	if matched := matchSavedScreen(app.Screen.GetAll(), layout); matched != nil {
		return matched
	}
	return app.Screen.GetPrimary()
}

func clampLayoutToScreen(layout *WindowLayout, screen *application.Screen) {
	var work application.Rect
	if screen != nil {
		work = screen.WorkArea
	}
	clampLayoutToWorkArea(layout, work)
}

func clampLayoutToWorkArea(layout *WindowLayout, work application.Rect) {
	if layout == nil {
		return
	}
	layout.Width, layout.Height = clampWindowSize(layout.Width, layout.Height)
	if layout.X < 0 {
		layout.X = 0
	}
	if layout.Y < 0 {
		layout.Y = 0
	}
	if work.Width > 0 && layout.Width > work.Width {
		layout.Width = work.Width
	}
	if work.Height > 0 && layout.Height > work.Height {
		layout.Height = work.Height
	}
	if work.Width > 0 && layout.X > work.Width-80 {
		layout.X = max(0, work.Width-80)
	}
	if work.Height > 0 && layout.Y > work.Height-80 {
		layout.Y = max(0, work.Height-80)
	}
}

// spawnWindowLayout 计算由 parent 打开的新窗口的位置：尺寸取工作区域的四分之三，
// 相对父窗口向右下方偏移一步居中，并保证完整落在工作区域内。
func spawnWindowLayout(parent WindowLayout, work application.Rect) WindowLayout {
	width, height := defaultWindowWidth, defaultWindowHeight
	if work.Width > 0 {
		width = int(float64(work.Width) * spawnWindowScreenPart)
	}
	if work.Height > 0 {
		height = int(float64(work.Height) * spawnWindowScreenPart)
	}
	width, height = clampWindowSize(width, height)

	out := WindowLayout{
		ScreenID:   parent.ScreenID,
		ScreenName: parent.ScreenName,
		X:          parent.X + parent.Width/2 + spawnWindowCenterStep - width/2,
		Y:          parent.Y + parent.Height/2 + spawnWindowCenterStep - height/2,
		Width:      width,
		Height:     height,
	}
	if work.Width > 0 {
		out.X = min(out.X, work.Width-width)
	}
	if work.Height > 0 {
		out.Y = min(out.Y, work.Height-height)
	}
	clampLayoutToWorkArea(&out, work)
	return out
}

func spawnLayoutFrom(app *application.App, parent application.Window) *WindowLayout {
	if app == nil || parent == nil {
		return nil
	}
	current := captureWindowLayout(parent)
	if current == nil {
		return nil
	}
	var work application.Rect
	if screen, err := parent.GetScreen(); err == nil && screen != nil {
		work = screen.WorkArea
	}
	out := spawnWindowLayout(*current, work)
	return &out
}

func applyWindowLayoutToOptions(opts *application.WebviewWindowOptions, layout *WindowLayout, app *application.App) {
	if opts == nil || layout == nil || layout.Width <= 0 || layout.Height <= 0 {
		return
	}
	screen := findScreen(app, layout)
	next := *layout
	clampLayoutToScreen(&next, screen)
	opts.Width = next.Width
	opts.Height = next.Height
	opts.X = next.X
	opts.Y = next.Y
	opts.InitialPosition = application.WindowXY
	if screen != nil {
		opts.Screen = screen
	}
	if next.Fullscreen {
		opts.StartState = application.WindowStateFullscreen
	} else if next.Maximized {
		opts.StartState = application.WindowStateMaximised
	}
}

func applyWindowLayoutToWindow(win application.Window, layout *WindowLayout, app *application.App) {
	if win == nil || layout == nil || layout.Width <= 0 || layout.Height <= 0 {
		return
	}
	screen := findScreen(app, layout)
	next := *layout
	clampLayoutToScreen(&next, screen)
	if win.IsFullscreen() {
		win.UnFullscreen()
	}
	if win.IsMaximised() {
		win.UnMaximise()
	}
	win.SetSize(next.Width, next.Height)
	if screen != nil {
		win.SetScreen(screen)
	}
	win.SetRelativePosition(next.X, next.Y)
	if next.Fullscreen {
		win.Fullscreen()
	} else if next.Maximized {
		win.Maximise()
	}
}

func captureWindowLayout(win application.Window) *WindowLayout {
	if win == nil {
		return nil
	}
	width, height := win.Size()
	if width <= 0 || height <= 0 {
		return nil
	}
	x, y := win.RelativePosition()
	out := &WindowLayout{
		X:          x,
		Y:          y,
		Width:      width,
		Height:     height,
		Maximized:  win.IsMaximised(),
		Fullscreen: win.IsFullscreen(),
	}
	if screen, err := win.GetScreen(); err == nil && screen != nil {
		out.ScreenID = screen.ID
		out.ScreenName = screen.Name
	}
	return out
}

func createWindow(app *application.App, svc *AppService, restore bool) application.Window {
	opts := defaultWindowOptions(windowURL(restore, nil))
	var startupLayout *WindowLayout
	if restore && svc != nil {
		startupLayout = svc.lastWorkspaceWindowLayout()
		applyWindowLayoutToOptions(&opts, startupLayout, app)
	}
	win := app.Window.NewWithOptions(opts)
	if svc != nil {
		svc.attachWindow(win, startupLayout, false)
	}
	return win
}

// openTargetWindow 在一个新窗口里展示 target，而不是占用用户发起打开操作的
// 那个窗口。
func openTargetWindow(app *application.App, svc *AppService, target *OpenTarget) application.Window {
	if app == nil || target == nil {
		return nil
	}
	opts := defaultWindowOptions(windowURL(false, target))
	layout := spawnLayoutFrom(app, app.Window.Current())
	applyWindowLayoutToOptions(&opts, layout, app)
	win := app.Window.NewWithOptions(opts)
	if svc != nil {
		svc.attachWindow(win, layout, true)
		svc.rememberWindowTarget(win.ID(), target)
	}
	return win
}

func setupMenu(app *application.App, svc *AppService) {
	menu := app.NewMenu()
	menu.AddRole(application.AppMenu)

	file := menu.AddSubmenu("文件")
	file.Add("新建窗口").SetAccelerator("CmdOrCtrl+Shift+n").OnClick(func(_ *application.Context) {
		createWindow(app, svc, false)
	})
	file.Add("打开文件…").SetAccelerator("CmdOrCtrl+o").OnClick(func(_ *application.Context) {
		target, err := svc.pickFile()
		if err != nil || target == nil {
			return
		}
		_ = svc.persist(target)
		openTargetWindow(app, svc, target)
	})
	file.Add("打开文件夹…").SetAccelerator("CmdOrCtrl+Shift+o").OnClick(func(_ *application.Context) {
		target, err := svc.pickDirectory()
		if err != nil || target == nil {
			return
		}
		_ = svc.persist(target)
		openTargetWindow(app, svc, target)
	})
	file.AddSeparator()
	file.Add("保存").SetAccelerator("CmdOrCtrl+s").OnClick(func(_ *application.Context) {
		if win := app.Window.Current(); win != nil {
			win.ExecJS(`window.dispatchEvent(new CustomEvent("veda:save"));`)
		}
	})
	file.AddSeparator()
	// 这里不能用 CloseWindow 角色：它自带的 CmdOrCtrl+W 会在按键到达页面之前就关掉
	// 窗口，而这个组合键现在由前端按设置里的“关闭标签页”快捷键处理。
	file.Add("关闭窗口").OnClick(func(_ *application.Context) {
		if win := app.Window.Current(); win != nil {
			win.Close()
		}
	})
	if runtime.GOOS != "darwin" {
		file.AddRole(application.Quit)
	}

	menu.AddRole(application.EditMenu)
	menu.AddRole(application.ViewMenu)
	if runtime.GOOS == "darwin" {
		menu.AddRole(application.WindowMenu)
	} else {
		// 非 macOS 的“窗口”角色菜单同样带一个 CmdOrCtrl+W 的关闭项，这里手工拼出
		// 不含它的版本，好让用户能把关闭标签页绑到 Ctrl+W 上。
		window := menu.AddSubmenu("窗口")
		window.AddRole(application.Minimise)
		window.AddRole(application.Zoom)
	}
	menu.AddRole(application.HelpMenu)

	app.Menu.SetApplicationMenu(menu)
}

// attachWindow 挂上窗口的状态记录逻辑，并在窗口显示到屏幕上后应用 startupLayout。
// 恢复出来的布局会服从操作系统在记录的显示器上给出的最大化状态；而为当前窗口
// 专门计算出来的布局（force）则始终优先。
func (s *AppService) attachWindow(win application.Window, startupLayout *WindowLayout, force bool) {
	if win == nil {
		return
	}
	s.rememberNormalBounds(win)
	applied := false
	applyStartup := func() {
		if applied || startupLayout == nil {
			return
		}
		app := s.currentApp()
		if app == nil {
			return
		}
		screens := app.Screen.GetAll()
		if len(screens) == 0 && app.Screen.GetPrimary() == nil {
			return
		}
		applied = true
		matched := matchSavedScreen(screens, startupLayout)
		if !force && (win.IsMaximised() || win.IsFullscreen()) && matched != nil {
			return
		}
		applyWindowLayoutToWindow(win, startupLayout, app)
	}
	win.OnWindowEvent(events.Common.WindowShow, func(_ *application.WindowEvent) {
		applyStartup()
	})
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(_ *application.WindowEvent) {
		applyStartup()
	})
	win.OnWindowEvent(events.Common.WindowDidMove, func(_ *application.WindowEvent) {
		s.rememberNormalBounds(win)
	})
	win.OnWindowEvent(events.Common.WindowDidResize, func(_ *application.WindowEvent) {
		s.rememberNormalBounds(win)
	})
	win.RegisterHook(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		s.persistWindow(win)
	})
}
