package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// AppService 是暴露给前端的 API。
type AppService struct {
	app           *application.App
	session       *sessionStore
	settings      *settingsStore
	locks         *lockStore
	layouts       *layoutStore
	updates       *updater
	mu            sync.Mutex
	roots         map[string]struct{}
	files         map[string]struct{}
	normalBounds  map[uint]WindowLayout
	windowTargets map[uint]OpenTarget

	stopOnce sync.Once
	stopCh   chan struct{}
}

func newAppService() *AppService {
	s := &AppService{
		session:       &sessionStore{},
		settings:      &settingsStore{},
		locks:         &lockStore{},
		layouts:       &layoutStore{},
		updates:       newUpdater(),
		roots:         map[string]struct{}{},
		files:         map[string]struct{}{},
		normalBounds:  map[uint]WindowLayout{},
		windowTargets: map[uint]OpenTarget{},
		stopCh:        make(chan struct{}),
	}
	_ = s.settings.ensureGlobal()
	return s
}

func init() {
	// wails3 generate 会把 package main 里的方法哈希成 "main.AppService.*"，
	// 而运行时用的是模块路径。这里固定住生成的 ID，保证前端绑定能解析到。
	application.RegisterBindingMethodID((*AppService).CheckForUpdate, 2651044200)
	application.RegisterBindingMethodID((*AppService).CloseWindow, 4083233194)
	application.RegisterBindingMethodID((*AppService).CopyFile, 1105439977)
	application.RegisterBindingMethodID((*AppService).CreateFolder, 3863529870)
	application.RegisterBindingMethodID((*AppService).CreateMarkdown, 2462821979)
	application.RegisterBindingMethodID((*AppService).Delete, 2416349173)
	application.RegisterBindingMethodID((*AppService).GetAppInfo, 4184398461)
	application.RegisterBindingMethodID((*AppService).GetLayout, 1732508512)
	application.RegisterBindingMethodID((*AppService).GetSession, 4287414074)
	application.RegisterBindingMethodID((*AppService).GetFileLock, 2172180487)
	application.RegisterBindingMethodID((*AppService).GetSettings, 3018893939)
	application.RegisterBindingMethodID((*AppService).GetUpdateState, 3686901152)
	application.RegisterBindingMethodID((*AppService).ImportImageData, 4168017972)
	application.RegisterBindingMethodID((*AppService).InstallUpdate, 3729456840)
	application.RegisterBindingMethodID((*AppService).MediaURL, 865123779)
	application.RegisterBindingMethodID((*AppService).MoveToTrash, 1780663512)
	application.RegisterBindingMethodID((*AppService).NewWindow, 168487400)
	application.RegisterBindingMethodID((*AppService).OpenURL, 790318107)
	application.RegisterBindingMethodID((*AppService).PickDirectory, 4292351642)
	application.RegisterBindingMethodID((*AppService).PickFile, 870419925)
	application.RegisterBindingMethodID((*AppService).ReadDir, 870691193)
	application.RegisterBindingMethodID((*AppService).ReadText, 3489741593)
	application.RegisterBindingMethodID((*AppService).Rename, 2637606580)
	application.RegisterBindingMethodID((*AppService).RevealInFileManager, 180266905)
	application.RegisterBindingMethodID((*AppService).ResolveImagePath, 2591528072)
	application.RegisterBindingMethodID((*AppService).ResolvePath, 3758697577)
	application.RegisterBindingMethodID((*AppService).SaveFileLock, 3362615718)
	application.RegisterBindingMethodID((*AppService).SaveSession, 1833592449)
	application.RegisterBindingMethodID((*AppService).SaveSettings, 3784651466)
	application.RegisterBindingMethodID((*AppService).SaveHeadingPanelWidth, 3419312925)
	application.RegisterBindingMethodID((*AppService).SaveOpenTabs, 1239682819)
	application.RegisterBindingMethodID((*AppService).SaveSidebarWidth, 1163423635)
	application.RegisterBindingMethodID((*AppService).Stat, 1965543488)
	application.RegisterBindingMethodID((*AppService).WriteText, 254991214)
	application.RegisterBindingMethodID((*AppService).SearchContent, 410169307)
	application.RegisterBindingMethodID((*AppService).SearchWorkspace, 2068548899)
}

func (s *AppService) shouldRestoreOnStartup() bool {
	return s.settings.shouldRestoreOnStartup()
}

func (s *AppService) attach(app *application.App) {
	s.app = app
	if app == nil {
		return
	}
	s.startUpdateScheduler()
	app.OnShutdown(func() {
		s.stopUpdateScheduler()
		s.persistWindowOnQuit()
	})
}

// stopUpdateScheduler 让自动检查更新的后台协程退出。
func (s *AppService) stopUpdateScheduler() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

// startUpdateScheduler 在启动时检查一次更新，之后每 6 小时检查一次。
// 只有全局设置里打开「自动检查更新」时才会真正发起检查。
func (s *AppService) startUpdateScheduler() {
	go func() {
		timer := time.NewTimer(updateInitialDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
			s.autoCheckUpdates()
		case <-s.stopCh:
			return
		}

		ticker := time.NewTicker(updateCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.autoCheckUpdates()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *AppService) autoCheckUpdates() {
	if !s.settings.autoCheckUpdates() {
		return
	}
	s.updates.Check()
}

func (s *AppService) currentApp() *application.App {
	if s.app != nil {
		return s.app
	}
	s.app = application.Get()
	return s.app
}

func (s *AppService) allow(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	if info.IsDir() {
		s.roots[abs] = struct{}{}
		return
	}
	s.files[abs] = struct{}{}
	s.roots[filepath.Dir(abs)] = struct{}{}
}

func (s *AppService) isAllowed(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.files[abs]; ok {
		return true
	}
	for root := range s.roots {
		rel, err := filepath.Rel(root, abs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (s *AppService) persist(target *OpenTarget) error {
	if target == nil || target.Path == "" {
		return nil
	}
	s.allow(target.Path)
	if target.File != "" {
		s.allow(target.File)
	}
	return s.session.Save(&Session{Last: target})
}

// GetSession 返回最近一次保存的工作区。
func (s *AppService) GetSession() (*Session, error) {
	sess, err := s.session.Load()
	if err != nil {
		return nil, err
	}
	if sess.Last != nil && sess.Last.Path != "" {
		if _, err := os.Stat(sess.Last.Path); err != nil {
			sess.Last = nil
			return sess, nil
		}
		s.allow(sess.Last.Path)
		if sess.Last.File != "" {
			if _, err := os.Stat(sess.Last.File); err == nil {
				s.allow(sess.Last.File)
			} else {
				sess.Last.File = ""
			}
		}
	}
	return sess, nil
}

// GetSettings 加载全局设置，以及可选的工作区设置。
func (s *AppService) GetSettings(workspacePath string) (*SettingsBundle, error) {
	return s.settings.loadBundle(workspacePath)
}

// GetAppInfo 返回「关于」页面需要的版本与平台信息。
func (s *AppService) GetAppInfo() *AppInfo {
	return &AppInfo{
		Version:     currentVersion(),
		Platform:    runtime.GOOS,
		Arch:        runtime.GOARCH,
		ReleasesURL: releasesPageURL,
	}
}

// GetUpdateState 返回更新检查与下载的当前状态。
func (s *AppService) GetUpdateState() *UpdateState {
	state := s.updates.State()
	if state.CurrentVersion == "" {
		state.CurrentVersion = currentVersion()
	}
	return &state
}

// CheckForUpdate 主动检查一次更新。检查与下载在后台进行，
// 前端通过 GetUpdateState 轮询进度。
func (s *AppService) CheckForUpdate() *UpdateState {
	state := s.updates.Check()
	if state.CurrentVersion == "" {
		state.CurrentVersion = currentVersion()
	}
	return &state
}

// InstallUpdate 安装已下载的更新并重启应用。
func (s *AppService) InstallUpdate() error {
	if err := s.updates.Install(); err != nil {
		return err
	}
	app := s.currentApp()
	if app == nil {
		return nil
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		app.Quit()
	}()
	return nil
}

// GetLayout 加载工作区的界面布局（侧边栏宽度与窗口几何信息）。
func (s *AppService) GetLayout(workspacePath string) (*WorkspaceLayout, error) {
	return s.layouts.load(workspacePath)
}

// SaveSidebarWidth 记录该工作区的目录树宽度。
func (s *AppService) SaveSidebarWidth(workspacePath string, width int) (*WorkspaceLayout, error) {
	return s.layouts.saveSidebarWidth(workspacePath, width)
}

// SaveHeadingPanelWidth 记录该工作区的 Markdown 大纲面板宽度。
func (s *AppService) SaveHeadingPanelWidth(workspacePath string, width int) (*WorkspaceLayout, error) {
	return s.layouts.saveHeadingPanelWidth(workspacePath, width)
}

// SaveOpenTabs 记录该工作区已打开的标签页路径（不含文件内容）。
func (s *AppService) SaveOpenTabs(workspacePath string, tabs []string, active string) (*WorkspaceLayout, error) {
	return s.layouts.saveOpenTabs(workspacePath, tabs, active)
}

// GetFileLock 返回文档已持久化的锁定状态，没有记录时默认为编辑模式。
func (s *AppService) GetFileLock(workspacePath, filePath string) (*FileLockState, error) {
	state, err := s.locks.get(workspacePath, filePath)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveFileLock 记录文档当前处于编辑模式还是只读模式。
func (s *AppService) SaveFileLock(workspacePath, filePath string, editing bool) error {
	return s.locks.save(workspacePath, filePath, editing)
}

// SaveSettings 以 JSON 形式写入一份设置文档。
func (s *AppService) SaveSettings(scope, workspacePath, settingsJSON string) (*SettingsBundle, error) {
	settings := DefaultSettings()
	if strings.TrimSpace(settingsJSON) != "" {
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			return nil, err
		}
	}
	return s.settings.save(scope, workspacePath, settings)
}

// SearchContent 在内存中的文本（含未保存的草稿）里匹配查询词。
func (s *AppService) SearchContent(content, query string, opts SearchOptions) (*SearchResult, error) {
	return searchContent(content, query, opts)
}

// SearchWorkspace 遍历文件夹，匹配 Markdown 及其他文本文件。
func (s *AppService) SearchWorkspace(ctx context.Context, workspacePath, query string, opts SearchOptions) (*SearchResult, error) {
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	return searchWorkspace(ctx, abs, query, opts)
}

// SaveSession 写入当前选定的工作区，并把它绑定到发起调用的窗口上，
// 这样窗口关闭时能把同一个工作区写回 session.json。
func (s *AppService) SaveSession(ctx context.Context, kind, path, file string) error {
	if kind != "file" && kind != "directory" {
		return errors.New("无效的打开类型")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	target := &OpenTarget{Kind: kind, Path: abs, File: file}
	if file != "" {
		if fAbs, err := filepath.Abs(file); err == nil {
			target.File = fAbs
		}
	}
	if win := callingWindow(ctx); win != nil {
		s.rememberWindowTarget(win.ID(), target)
	}
	return s.persist(target)
}

// callingWindow 返回绑定方法是从哪个窗口发起调用的。
func callingWindow(ctx context.Context) application.Window {
	if ctx == nil {
		return nil
	}
	win, _ := ctx.Value(application.WindowKey).(application.Window)
	return win
}

func (s *AppService) rememberWindowTarget(windowID uint, target *OpenTarget) {
	if target == nil || target.Path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.windowTargets == nil {
		s.windowTargets = map[uint]OpenTarget{}
	}
	s.windowTargets[windowID] = *target
}

func (s *AppService) windowTarget(windowID uint) *OpenTarget {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := s.windowTargets[windowID]
	if !ok {
		return nil
	}
	return &target
}

func (s *AppService) pickFile() (*OpenTarget, error) {
	app := s.currentApp()
	dialog := app.Dialog.OpenFile().
		SetTitle("打开文件").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		AddFilter("文本与 Markdown", "*.md;*.markdown;*.txt;*.text").
		AddFilter("所有文件", "*.*")
	if win := app.Window.Current(); win != nil {
		dialog.AttachToWindow(win)
	}
	path, err := dialog.PromptForSingleSelection()
	if err != nil || path == "" {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	target := &OpenTarget{Kind: "file", Path: abs}
	s.allow(abs)
	return target, nil
}

func (s *AppService) pickDirectory() (*OpenTarget, error) {
	app := s.currentApp()
	dialog := app.Dialog.OpenFile().
		SetTitle("打开文件夹").
		CanChooseFiles(false).
		CanChooseDirectories(true)
	if win := app.Window.Current(); win != nil {
		dialog.AttachToWindow(win)
	}
	path, err := dialog.PromptForSingleSelection()
	if err != nil || path == "" {
		return nil, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	target := &OpenTarget{Kind: "directory", Path: abs}
	s.allow(abs)
	return target, nil
}

// PickFile 打开系统原生的文件选择对话框。
func (s *AppService) PickFile() (*OpenTarget, error) {
	target, err := s.pickFile()
	if err != nil || target == nil {
		return target, err
	}
	_ = s.persist(target)
	return target, nil
}

// PickDirectory 打开系统原生的文件夹选择对话框。
func (s *AppService) PickDirectory() (*OpenTarget, error) {
	target, err := s.pickDirectory()
	if err != nil || target == nil {
		return target, err
	}
	_ = s.persist(target)
	s.applyWorkspaceWindow(target.Path)
	return target, nil
}

// NewWindow 再打开一个应用窗口。
func (s *AppService) NewWindow() {
	createWindow(s.currentApp(), s, false)
}

// CloseWindow 关闭当前窗口。前端在没有标签页可关的时候调用它。
func (s *AppService) CloseWindow() {
	app := s.currentApp()
	if app == nil {
		return
	}
	if win := app.Window.Current(); win != nil {
		win.Close()
	}
}

// OpenURL 用系统默认浏览器打开链接。
func (s *AppService) OpenURL(raw string) error {
	target := strings.TrimSpace(raw)
	if target == "" || strings.HasPrefix(target, "#") {
		return nil
	}
	if !strings.Contains(target, "://") && !strings.HasPrefix(strings.ToLower(target), "mailto:") {
		target = "https://" + target
	}
	sanitized, err := application.ValidateAndSanitizeURL(target)
	if err != nil {
		return err
	}
	app := s.currentApp()
	if app == nil {
		return errors.New("应用未就绪")
	}
	return app.Browser.OpenURL(sanitized)
}

// Stat 返回某个路径的元信息。
func (s *AppService) Stat(path string) (*FileMeta, error) {
	return fileMeta(path)
}

// ReadDir 列出目录内容。
func (s *AppService) ReadDir(ctx context.Context, path string) ([]FileEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	visibleResourcePath := ""
	if win := callingWindow(ctx); win != nil {
		if target := s.windowTarget(win.ID()); target != nil && target.Kind == "directory" {
			if bundle, loadErr := s.settings.loadBundle(target.Path); loadErr == nil &&
				bundle.Workspace.General.ShowResourceDirectory {
				visibleResourcePath = filepath.Join(
					target.Path,
					normalizeResourceDirectory(bundle.Workspace.General.ResourceDirectory),
				)
			}
		}
	}
	return readDirEntriesWithVisiblePath(abs, visibleResourcePath)
}

// ReadText 读取 Markdown 或纯文本文件。
func (s *AppService) ReadText(path string) (*TextFile, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	return readTextFile(abs)
}

// WriteText 保存文本文件，不会改动操作系统层面的文件权限。
func (s *AppService) WriteText(path, content string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !s.isAllowed(abs) {
		s.allow(abs)
	}
	if err := writeTextFile(abs, content); err != nil {
		return err
	}
	return nil
}

// CreateFolder 在 parent 下创建一个不重名的文件夹。
func (s *AppService) CreateFolder(parent, name string) (*FileEntry, error) {
	abs, err := filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	entry, err := createFolder(abs, name)
	if err != nil {
		return nil, err
	}
	s.allow(entry.Path)
	return entry, nil
}

// CreateMarkdown 在 parent 下创建一个不重名的 Markdown 文件。
func (s *AppService) CreateMarkdown(parent, name string) (*FileEntry, error) {
	abs, err := filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	entry, err := createMarkdown(abs, name)
	if err != nil {
		return nil, err
	}
	s.allow(entry.Path)
	return entry, nil
}

// Rename 原地重命名文件或文件夹。
func (s *AppService) Rename(path, newName string) (*FileEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	entry, err := renameEntry(abs, newName)
	if err != nil {
		return nil, err
	}
	s.rewriteAllowed(abs, entry.Path)
	s.rewriteConfigs(abs, entry.Path)
	s.allow(entry.Path)
	return entry, nil
}

// Delete 永久删除文件或文件夹，当前工作区根目录不允许删除。
func (s *AppService) Delete(path string) error {
	return s.removeEntry(path, deleteEntry)
}

// MoveToTrash 把文件或文件夹交给系统回收站处理。
func (s *AppService) MoveToTrash(path string) error {
	return s.removeEntry(path, moveToTrash)
}

func (s *AppService) removeEntry(path string, remove func(string) error) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	s.mu.Lock()
	for _, target := range s.windowTargets {
		if target.Kind == "directory" && target.Path == abs {
			s.mu.Unlock()
			return errors.New("不能删除当前工作区")
		}
	}
	s.mu.Unlock()
	if err := remove(abs); err != nil {
		return err
	}
	s.forgetAllowed(abs)
	return nil
}

// RevealInFileManager 在 Finder、资源管理器或系统文件管理器中定位文件或文件夹。
func (s *AppService) RevealInFileManager(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	s.allow(abs)
	return revealInFileManager(abs)
}

// CopyFile 把文件或文件夹放进系统剪贴板，以便在文件管理器里粘贴。
func (s *AppService) CopyFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	s.allow(abs)
	return copyFileToClipboard(abs)
}

func (s *AppService) lastWorkspaceWindowLayout() *WindowLayout {
	sess, err := s.session.Load()
	if err != nil || sess == nil || sess.Last == nil || sess.Last.Kind != "directory" || sess.Last.Path == "" {
		return nil
	}
	layout, err := s.layouts.load(sess.Last.Path)
	if err != nil || layout == nil || layout.Window == nil {
		return nil
	}
	return layout.Window
}

func (s *AppService) applyWorkspaceWindow(workspacePath string) {
	if workspacePath == "" {
		return
	}
	layout, err := s.layouts.load(workspacePath)
	if err != nil || layout == nil || layout.Window == nil {
		return
	}
	app := s.currentApp()
	if app == nil {
		return
	}
	applyWindowLayoutToWindow(app.Window.Current(), layout.Window, app)
}

func (s *AppService) rememberNormalBounds(win application.Window) {
	if win == nil || win.IsMaximised() || win.IsFullscreen() || win.IsMinimised() {
		return
	}
	layout := captureWindowLayout(win)
	if layout == nil {
		return
	}
	s.mu.Lock()
	if s.normalBounds == nil {
		s.normalBounds = map[uint]WindowLayout{}
	}
	s.normalBounds[win.ID()] = *layout
	s.mu.Unlock()
}

func (s *AppService) captureWindowForSave(win application.Window) *WindowLayout {
	current := captureWindowLayout(win)
	if current == nil {
		return nil
	}
	if current.Maximized || current.Fullscreen {
		s.mu.Lock()
		normal, ok := s.normalBounds[win.ID()]
		s.mu.Unlock()
		if ok && normal.Width > 0 && normal.Height > 0 {
			current.X = normal.X
			current.Y = normal.Y
			current.Width = normal.Width
			current.Height = normal.Height
			if current.ScreenID == "" {
				current.ScreenID = normal.ScreenID
				current.ScreenName = normal.ScreenName
			}
		}
	}
	return current
}

// saveSessionTarget 把 target 设为下次启动时要恢复的工作区。
func (s *AppService) saveSessionTarget(target *OpenTarget) {
	if target == nil {
		return
	}
	_ = s.session.Save(&Session{Last: target})
}

// persistWindow 把该窗口的工作区写入全局会话。每次关闭都会覆盖这个文件，
// 所以最后关闭的那个窗口决定了下次启动恢复什么。
func (s *AppService) persistWindow(win application.Window) {
	if win == nil {
		return
	}
	target := s.windowTarget(win.ID())
	if target == nil {
		return
	}
	if target.Kind == "directory" {
		if layout := s.captureWindowForSave(win); layout != nil {
			_ = s.layouts.saveWindow(target.Path, layout)
		}
	}
	s.saveSessionTarget(target)
}

func (s *AppService) persistWindowOnQuit() {
	app := s.currentApp()
	if app == nil {
		return
	}
	win := app.Window.Current()
	if win == nil {
		windows := app.Window.GetAll()
		if len(windows) == 0 {
			return
		}
		win = windows[0]
	}
	s.persistWindow(win)
}

func (s *AppService) rewriteConfigs(oldPath, newPath string) {
	if oldPath == newPath {
		return
	}
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	s.settings.mu.Lock()
	defer s.settings.mu.Unlock()
	s.locks.mu.Lock()
	defer s.locks.mu.Unlock()
	s.layouts.mu.Lock()
	defer s.layouts.mu.Unlock()

	s.mu.Lock()
	roots := make([]string, 0, len(s.roots))
	for root := range s.roots {
		roots = append(roots, root)
	}
	s.mu.Unlock()
	_ = rewriteVedaConfigs(oldPath, newPath, roots)
}

func (s *AppService) rewriteAllowed(oldPath, newPath string) {
	if oldPath == newPath {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rewrite := func(m map[string]struct{}) {
		next := make([]string, 0)
		prefix := oldPath + string(os.PathSeparator)
		for p := range m {
			if p == oldPath || strings.HasPrefix(p, prefix) {
				delete(m, p)
				next = append(next, newPath+p[len(oldPath):])
			}
		}
		for _, p := range next {
			m[p] = struct{}{}
		}
	}
	rewrite(s.roots)
	rewrite(s.files)
}

func (s *AppService) forgetAllowed(oldPath string) {
	if oldPath == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := oldPath + string(os.PathSeparator)
	for _, m := range []map[string]struct{}{s.roots, s.files} {
		for p := range m {
			if p == oldPath || strings.HasPrefix(p, prefix) {
				delete(m, p)
			}
		}
	}
}

// ResolvePath 把目录和一个（可能是相对的）路径拼接起来。
func (s *AppService) ResolvePath(fromDir, src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", errors.New("空路径")
	}
	if decoded, err := url.PathUnescape(src); err == nil {
		src = decoded
	}
	if filepath.IsAbs(src) {
		return filepath.Clean(src), nil
	}
	return filepath.Clean(filepath.Join(fromDir, src)), nil
}

func existingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func resourcePathTail(src, resourceDir string) string {
	clean := filepath.Clean(filepath.FromSlash(src))
	resource := filepath.Clean(filepath.FromSlash(resourceDir))
	if rel, err := filepath.Rel(resource, clean); err == nil &&
		rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return rel
	}
	return clean
}

// ResolveImagePath 解析工作区内的资源链接，并在单文件模式下按回退规则查找。
func (s *AppService) ResolveImagePath(workspacePath, fromDir, src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", errors.New("空路径")
	}
	if decoded, err := url.PathUnescape(src); err == nil {
		src = decoded
	}
	if filepath.IsAbs(src) {
		return filepath.Clean(src), nil
	}

	direct := filepath.Clean(filepath.Join(fromDir, filepath.FromSlash(src)))
	if existingFile(direct) {
		return direct, nil
	}
	bundle, err := s.settings.loadBundle(workspacePath)
	if err != nil {
		return "", err
	}
	if workspacePath != "" {
		workspaceCandidate := filepath.Clean(filepath.Join(workspacePath, filepath.FromSlash(src)))
		return workspaceCandidate, nil
	}

	resourceDir := normalizeResourceDirectory(bundle.Global.General.ResourceDirectory)
	tail := resourcePathTail(src, resourceDir)
	current, err := filepath.Abs(fromDir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(current, resourceDir, tail)
		if existingFile(candidate) {
			return candidate, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	globalDir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(globalDir, resourceDir, tail), nil
}

// MediaURL 返回本地媒体文件对应的资源服务器 URL。
func (s *AppService) MediaURL(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	s.allow(abs)
	enc := base64.RawURLEncoding.EncodeToString([]byte(abs))
	return "/__media/" + enc, nil
}

func (s *AppService) mediaHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/__media/") {
			next.ServeHTTP(w, r)
			return
		}
		enc := strings.TrimPrefix(r.URL.Path, "/__media/")
		raw, err := base64.RawURLEncoding.DecodeString(enc)
		if err != nil {
			http.Error(w, "bad media id", http.StatusBadRequest)
			return
		}
		path := string(raw)
		if !s.isAllowed(path) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		kind := classifyFile(path, false)
		if kind != KindImage && kind != KindSVG && kind != KindVideo {
			http.Error(w, "unsupported", http.StatusUnsupportedMediaType)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			http.Error(w, "unreadable", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", mimeForExt(path))
		w.Header().Set("Cache-Control", "private, max-age=60")
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	})
}
