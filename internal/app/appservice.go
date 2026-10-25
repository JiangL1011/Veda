// Package app 是 Veda 的 Wails 服务层：向前端暴露 AppService 的方法，
// 并负责窗口、菜单、预览路由、拖拽导入与资源保存。
package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"veda/internal/files"
	"veda/internal/model"
	"veda/internal/search"
	"veda/internal/store"
	"veda/internal/update"
)

// AppService 是暴露给前端的 API。
type AppService struct {
	app           *application.App
	store         *store.Store
	updates       *update.Updater
	version       func() string
	mu            sync.Mutex
	roots         map[string]struct{}
	files         map[string]struct{}
	normalBounds  map[uint]model.WindowLayout
	windowTargets map[uint]model.OpenTarget
	pendingDrops  map[uint]*pendingFileDrop

	stopOnce sync.Once
	stopCh   chan struct{}
}

// pendingFileDrop 是一次等待用户选择「复制还是移动」的拖拽导入。
type pendingFileDrop struct {
	files     []string
	destDir   string
	workspace string
}

// New 创建暴露给前端的服务实例。currentVersion 返回当前应用版本，
// 用于“关于”页面和更新检查。
func New(currentVersion func() string) *AppService {
	s := &AppService{
		store:         store.New(),
		updates:       update.New(currentVersion, store.DataDir),
		version:       currentVersion,
		roots:         map[string]struct{}{},
		files:         map[string]struct{}{},
		normalBounds:  map[uint]model.WindowLayout{},
		windowTargets: map[uint]model.OpenTarget{},
		pendingDrops:  map[uint]*pendingFileDrop{},
		stopCh:        make(chan struct{}),
	}
	_ = s.store.Settings.EnsureGlobal()
	return s
}

func (s *AppService) shouldRestoreOnStartup() bool {
	return s.store.Settings.ShouldRestoreOnStartup()
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

// Attach 把服务挂到正在运行的 Wails 应用上。
func Attach(s *AppService, app *application.App) {
	s.attach(app)
}

// Persist 把 target 记为下次启动时要恢复的工作区。
func Persist(s *AppService, target *model.OpenTarget) error {
	return s.persist(target)
}

// ShouldRestoreOnStartup 报告启动时是否应该恢复上次的工作区。
func ShouldRestoreOnStartup(s *AppService) bool {
	return s.shouldRestoreOnStartup()
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
	// 启动时先回收上一次运行留下的安装包与助手临时目录：此刻不可能有正在进行的安装
	// （安装助手要把应用重新拉起来，才会轮到新进程）。
	s.updates.CleanupStaleDownloads()
	go func() {
		timer := time.NewTimer(update.InitialDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
			s.autoCheckUpdates()
		case <-s.stopCh:
			return
		}

		ticker := time.NewTicker(update.CheckInterval)
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
	if !s.store.Settings.AutoCheckUpdates() {
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

func (s *AppService) persist(target *model.OpenTarget) error {
	if target == nil || target.Path == "" {
		return nil
	}
	s.allow(target.Path)
	if target.File != "" {
		s.allow(target.File)
	}
	// 迁移期间不写 session：写入会等待迁移结束，可能挡住窗口显示。
	// 前端在迁移完成后调用 SaveSession 会补上这次打开操作。
	if store.MigrationInProgress() {
		return nil
	}
	return s.store.Session.Save(&model.Session{Last: target})
}

// GetSession 返回最近一次保存的工作区。
func (s *AppService) GetSession() (*model.Session, error) {
	sess, err := s.store.Session.Load()
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
func (s *AppService) GetSettings(workspacePath string) (*model.SettingsBundle, error) {
	return s.store.Settings.LoadBundle(workspacePath)
}

// GetAppInfo 返回「关于」页面需要的版本与平台信息。
func (s *AppService) GetAppInfo() *model.AppInfo {
	return &model.AppInfo{
		Version:     s.version(),
		Platform:    runtime.GOOS,
		Arch:        runtime.GOARCH,
		ReleasesURL: update.ReleasesPageURL,
	}
}

// GetUpdateState 返回更新检查与下载的当前状态。
func (s *AppService) GetUpdateState() *model.UpdateState {
	state := s.updates.State()
	if state.CurrentVersion == "" {
		state.CurrentVersion = s.version()
	}
	return &state
}

// CheckForUpdate 主动检查一次更新。检查与下载在后台进行，
// 前端通过 GetUpdateState 轮询进度。手动检查会尽量拿到一次真实的结果。
func (s *AppService) CheckForUpdate() *model.UpdateState {
	state := s.updates.CheckManually()
	if state.CurrentVersion == "" {
		state.CurrentVersion = s.version()
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
func (s *AppService) GetLayout(workspacePath string) (*model.WorkspaceLayout, error) {
	return s.store.Layouts.Load(workspacePath)
}

// SaveSidebarWidth 记录该工作区的目录树宽度。
func (s *AppService) SaveSidebarWidth(workspacePath string, width int) (*model.WorkspaceLayout, error) {
	return s.store.Layouts.SaveSidebarWidth(workspacePath, width)
}

// SaveHeadingPanelWidth 记录该工作区的 Markdown 大纲面板宽度。
func (s *AppService) SaveHeadingPanelWidth(workspacePath string, width int) (*model.WorkspaceLayout, error) {
	return s.store.Layouts.SaveHeadingPanelWidth(workspacePath, width)
}

// SaveOpenTabs 记录该工作区已打开的标签页路径（不含文件内容）。
func (s *AppService) SaveOpenTabs(workspacePath string, tabs []string, active string) (*model.WorkspaceLayout, error) {
	return s.store.Layouts.SaveOpenTabs(workspacePath, tabs, active)
}

// GetFileLock 返回文档已持久化的锁定状态，没有记录时默认为编辑模式。
func (s *AppService) GetFileLock(workspacePath, filePath string) (*model.FileLockState, error) {
	state, err := s.store.Locks.Get(workspacePath, filePath)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveFileLock 记录文档当前处于编辑模式还是只读模式。
func (s *AppService) SaveFileLock(workspacePath, filePath string, editing bool) error {
	return s.store.Locks.Save(workspacePath, filePath, editing)
}

// SaveSettings 以 JSON 形式写入一份设置文档。
func (s *AppService) SaveSettings(scope, workspacePath, settingsJSON string) (*model.SettingsBundle, error) {
	settings := store.DefaultSettings()
	if strings.TrimSpace(settingsJSON) != "" {
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			return nil, err
		}
	}
	return s.store.Settings.Save(scope, workspacePath, settings)
}

// SearchContent 在内存中的文本（含未保存的草稿）里匹配查询词。
func (s *AppService) SearchContent(content, query string, opts model.SearchOptions) (*model.SearchResult, error) {
	return search.Content(content, query, opts)
}

// SearchWorkspace 遍历文件夹，匹配 Markdown 及其他文本文件。
func (s *AppService) SearchWorkspace(ctx context.Context, workspacePath, query string, opts model.SearchOptions) (*model.SearchResult, error) {
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	return search.Workspace(ctx, abs, query, opts)
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
	target := &model.OpenTarget{Kind: kind, Path: abs, File: file}
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

func (s *AppService) rememberWindowTarget(windowID uint, target *model.OpenTarget) {
	if target == nil || target.Path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.windowTargets == nil {
		s.windowTargets = map[uint]model.OpenTarget{}
	}
	s.windowTargets[windowID] = *target
}

func (s *AppService) windowTarget(windowID uint) *model.OpenTarget {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := s.windowTargets[windowID]
	if !ok {
		return nil
	}
	return &target
}

// rememberPendingDrop 记下某个窗口里等待用户选择处理方式的拖拽导入。
// 同一个窗口再次拖入会覆盖上一次还没处理完的记录。
func (s *AppService) rememberPendingDrop(windowID uint, drop *pendingFileDrop) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingDrops == nil {
		s.pendingDrops = map[uint]*pendingFileDrop{}
	}
	s.pendingDrops[windowID] = drop
}

func (s *AppService) takePendingDrop(windowID uint) *pendingFileDrop {
	s.mu.Lock()
	defer s.mu.Unlock()
	drop, ok := s.pendingDrops[windowID]
	if !ok {
		return nil
	}
	delete(s.pendingDrops, windowID)
	return drop
}

// dropDirAttribute 是前端标在拖拽落点上的目标目录属性名。
const dropDirAttribute = "data-drop-dir"

// handleWindowFilesDropped 处理从系统里拖进窗口的文件。单文件窗口（以及还没打开
// 工作区的窗口）不做任何复制或移动，直接在独立窗口里打开。
func (s *AppService) handleWindowFilesDropped(win application.Window, event *application.WindowEvent) {
	if win == nil || event == nil {
		return
	}
	files := event.Context().DroppedFiles()
	if len(files) == 0 {
		return
	}
	hintDir := ""
	if details := event.Context().DropTargetDetails(); details != nil {
		hintDir = strings.TrimSpace(details.Attributes[dropDirAttribute])
	}
	s.beginFileDrop(win, files, hintDir)
}

func (s *AppService) beginFileDrop(win application.Window, files []string, hintDir string) {
	target := s.windowTarget(win.ID())
	if target == nil || target.Kind != "directory" {
		s.openDroppedTargets(win, files)
		return
	}
	workspace := target.Path
	destDir := s.resolveDropDir(workspace, hintDir)
	mode := s.dragDropMode(workspace)
	if mode == model.DragDropCopy || mode == model.DragDropMove {
		s.applyFileDrop(win, files, destDir, workspace, mode == model.DragDropMove)
		return
	}
	s.rememberPendingDrop(win.ID(), &pendingFileDrop{files: files, destDir: destDir, workspace: workspace})
	emitWindowEvent(win, "veda:file-drop", map[string]any{
		"files":     files,
		"destDir":   destDir,
		"workspace": workspace,
	})
}

// openDroppedTargets 为拖进来的每一项新开一个窗口：文件按单文件模式打开，
// 文件夹按工作区打开，都不改动磁盘上的原件。
func (s *AppService) openDroppedTargets(parent application.Window, files []string) {
	app := s.currentApp()
	if app == nil {
		return
	}
	for _, file := range files {
		abs, err := filepath.Abs(file)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil {
			continue
		}
		s.allow(abs)
		_, _ = store.EnsureFileID(abs)
		target := &model.OpenTarget{Kind: "file", Path: abs}
		if info.IsDir() {
			target.Kind = "directory"
		}
		_ = s.persist(target)
		OpenTargetWindowFrom(app, s, parent, target)
	}
}

// resolveDropDir 把落点给出的目录收敛成工作区内的一个真实目录，越界或已不存在
// 时回到工作区根目录。
func (s *AppService) resolveDropDir(workspace, hintDir string) string {
	root, err := filepath.Abs(workspace)
	if err != nil {
		return workspace
	}
	if hintDir == "" {
		return root
	}
	candidate, err := filepath.Abs(hintDir)
	if err != nil || !files.IsSameOrInside(root, candidate) {
		return root
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return root
	}
	return candidate
}

// dragDropMode 读取当前工作区生效的拖拽处理方式，没有工作区覆盖层时沿用全局设置。
func (s *AppService) dragDropMode(workspacePath string) string {
	bundle, err := s.store.Settings.LoadBundle(workspacePath)
	if err != nil || bundle == nil {
		return model.DragDropAsk
	}
	switch bundle.Workspace.General.DragDropMode {
	case model.DragDropCopy:
		return model.DragDropCopy
	case model.DragDropMove:
		return model.DragDropMove
	default:
		return model.DragDropAsk
	}
}

func (s *AppService) applyFileDrop(win application.Window, paths []string, destDir, workspace string, move bool) *model.FileDropOutcome {
	action := model.DragDropCopy
	if move {
		action = model.DragDropMove
	}
	result := files.ImportPathsInto(paths, destDir, move)
	outcome := &model.FileDropOutcome{
		DestDir:   destDir,
		Workspace: workspace,
		Action:    action,
		Imported:  result.Entries,
		Errors:    result.Errors,
	}
	for i, entry := range result.Entries {
		s.allow(entry.Path)
		source := ""
		if i < len(result.Sources) {
			source = result.Sources[i]
		}
		if source == "" {
			_ = store.EnsureFileTree(entry.Path)
			continue
		}
		_ = store.TransferFileIndex(source, entry.Path, move)
	}
	if move {
		for _, path := range paths {
			if abs, err := filepath.Abs(path); err == nil {
				s.forgetAllowed(abs)
			}
		}
	}
	if win != nil {
		emitWindowEvent(win, "veda:file-drop-done", outcome)
	}
	return outcome
}

// ResolveFileDrop 处理拖拽文件后弹出的选择框：copy / move 真正落到磁盘，
// cancel 只是丢弃这次待处理的导入。
func (s *AppService) ResolveFileDrop(ctx context.Context, action string) (*model.FileDropOutcome, error) {
	win := callingWindow(ctx)
	if win == nil {
		return nil, errors.New("找不到发起调用的窗口")
	}
	drop := s.takePendingDrop(win.ID())
	if drop == nil {
		return nil, errors.New("没有等待处理的拖拽文件")
	}
	if action == "cancel" {
		return nil, nil
	}
	if action != model.DragDropCopy && action != model.DragDropMove {
		s.rememberPendingDrop(win.ID(), drop)
		return nil, errors.New("无效的处理方式")
	}
	return s.applyFileDrop(win, drop.files, drop.destDir, drop.workspace, action == model.DragDropMove), nil
}

// emitWindowEvent 只往指定窗口派发一个 CustomEvent；Go 的自定义事件会广播到所有
// 窗口，而拖拽导入只发生在发起拖拽的那个窗口里。
func emitWindowEvent(win application.Window, name string, payload any) {
	if win == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	win.ExecJS(fmt.Sprintf("window.dispatchEvent(new CustomEvent(%q, { detail: %s }));", name, string(raw)))
}

func (s *AppService) pickFile() (*model.OpenTarget, error) {
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
	target := &model.OpenTarget{Kind: "file", Path: abs}
	s.allow(abs)
	_, _ = store.EnsureFileID(abs)
	return target, nil
}

func (s *AppService) pickDirectory() (*model.OpenTarget, error) {
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
	target := &model.OpenTarget{Kind: "directory", Path: abs}
	s.allow(abs)
	_, _ = store.EnsureFileID(abs)
	return target, nil
}

// PickFile 打开系统原生的文件选择对话框。
func (s *AppService) PickFile() (*model.OpenTarget, error) {
	target, err := s.pickFile()
	if err != nil || target == nil {
		return target, err
	}
	_ = s.persist(target)
	return target, nil
}

// PickDirectory 打开系统原生的文件夹选择对话框。
func (s *AppService) PickDirectory() (*model.OpenTarget, error) {
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
	CreateWindow(s.currentApp(), s, false)
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
func (s *AppService) Stat(path string) (*model.FileMeta, error) {
	meta, err := files.FileMeta(path)
	if err == nil && meta != nil && meta.Exists {
		_, _ = store.EnsureFileID(meta.Path)
	}
	return meta, err
}

// ReadDir 列出目录内容。
func (s *AppService) ReadDir(ctx context.Context, path string) ([]model.FileEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	visibleResourcePath := ""
	if win := callingWindow(ctx); win != nil {
		if target := s.windowTarget(win.ID()); target != nil && target.Kind == "directory" {
			if bundle, loadErr := s.store.Settings.LoadBundle(target.Path); loadErr == nil &&
				bundle.Workspace.General.ShowResourceDirectory {
				visibleResourcePath = filepath.Join(
					target.Path,
					store.NormalizeResourceDirectory(bundle.Workspace.General.ResourceDirectory),
				)
			}
		}
	}
	entries, err := files.ReadDirEntriesWithVisiblePath(abs, visibleResourcePath)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries)+1)
	paths = append(paths, abs)
	for _, entry := range entries {
		paths = append(paths, entry.Path)
	}
	_, _ = store.EnsureFileIDs(paths)
	return entries, nil
}

// ReadText 读取 Markdown 或纯文本文件。
func (s *AppService) ReadText(path string) (*model.TextFile, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	file, err := files.ReadText(abs)
	if err != nil {
		return nil, err
	}
	_, _ = store.EnsureFileID(abs)
	return file, nil
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
	if err := files.WriteText(abs, content); err != nil {
		return err
	}
	_, _ = store.EnsureFileID(abs)
	return nil
}

// CreateFolder 在 parent 下创建一个不重名的文件夹。
func (s *AppService) CreateFolder(parent, name string) (*model.FileEntry, error) {
	abs, err := filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	_, _ = store.EnsureFileID(abs)
	entry, err := files.CreateFolder(abs, name)
	if err != nil {
		return nil, err
	}
	s.allow(entry.Path)
	_, _ = store.EnsureFileID(entry.Path)
	return entry, nil
}

// CreateDocument 在 parent 下创建一个不重名的文本文件。后缀由 name 决定，
// 可以不带后缀，非文本后缀会被拒绝。
func (s *AppService) CreateDocument(parent, name string) (*model.FileEntry, error) {
	abs, err := filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	_, _ = store.EnsureFileID(abs)
	entry, err := files.CreateDocument(abs, name)
	if err != nil {
		return nil, err
	}
	s.allow(entry.Path)
	_, _ = store.EnsureFileID(entry.Path)
	return entry, nil
}

// Rename 原地重命名文件或文件夹。
func (s *AppService) Rename(path, newName string) (*model.FileEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	s.allow(abs)
	entry, err := files.RenameEntry(abs, newName)
	if err != nil {
		return nil, err
	}
	// 编号跟着文件走。目录的子项也一并迁移，然后再补登记还没进索引的文件。
	_ = store.MoveFileIndex(abs, entry.Path)
	s.rewriteAllowed(abs, entry.Path)
	s.rewriteConfigs(abs, entry.Path)
	_ = store.EnsureFileTree(entry.Path)
	s.allow(entry.Path)
	return entry, nil
}

// Move 把文件或文件夹移动到 destDir。目录树里拖拽条目时调用它；磁盘上的
// 移动完成后，文件编号与各种配置里的路径会一起迁移。
func (s *AppService) Move(path, destDir string) (*model.FileEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dest, err := filepath.Abs(destDir)
	if err != nil {
		return nil, err
	}
	if !s.isAllowed(abs) || !s.isAllowed(dest) {
		return nil, errors.New("没有权限移动该文件")
	}
	entry, err := files.MoveEntryTo(abs, dest)
	if err != nil {
		return nil, err
	}
	if entry.Path == abs {
		// 拖回原来的目录：没有发生移动，索引和配置都不用改。
		return entry, nil
	}
	// 编号跟着文件（以及它下面的所有子项）走。
	_ = store.MoveFileIndex(abs, entry.Path)
	s.rewriteAllowed(abs, entry.Path)
	s.rewriteConfigs(abs, entry.Path)
	_ = store.EnsureFileTree(entry.Path)
	s.allow(entry.Path)
	return entry, nil
}

// Delete 永久删除文件或文件夹，当前工作区根目录不允许删除。
func (s *AppService) Delete(path string) error {
	return s.removeEntry(path, files.DeleteEntry, false)
}

// MoveToTrash 把文件或文件夹交给系统回收站处理。
// 回收站里的文件可能被还原，所以保留 file-index.json 里的编号；
// 还原到原路径后仍然是同一个文件身份，标签页和锁定状态不会丢。
func (s *AppService) MoveToTrash(path string) error {
	return s.removeEntry(path, files.MoveToTrash, true)
}

func (s *AppService) removeEntry(path string, remove func(string) error, preserveIndex bool) error {
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
	if !preserveIndex {
		_ = store.RemoveFileIndex(abs)
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
	return files.RevealInFileManager(abs)
}

// CopyFile 把文件或文件夹放进系统剪贴板，以便在文件管理器里粘贴。
func (s *AppService) CopyFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	s.allow(abs)
	return files.CopyFileToClipboard(abs)
}

func (s *AppService) lastWorkspaceWindowLayout() *model.WindowLayout {
	sess, err := s.store.Session.Load()
	if err != nil || sess == nil || sess.Last == nil || sess.Last.Kind != "directory" || sess.Last.Path == "" {
		return nil
	}
	layout, err := s.store.Layouts.Load(sess.Last.Path)
	if err != nil || layout == nil || layout.Window == nil {
		return nil
	}
	return layout.Window
}

func (s *AppService) applyWorkspaceWindow(workspacePath string) {
	if workspacePath == "" {
		return
	}
	layout, err := s.store.Layouts.Load(workspacePath)
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
		s.normalBounds = map[uint]model.WindowLayout{}
	}
	s.normalBounds[win.ID()] = *layout
	s.mu.Unlock()
}

func (s *AppService) captureWindowForSave(win application.Window) *model.WindowLayout {
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
func (s *AppService) saveSessionTarget(target *model.OpenTarget) {
	if target == nil {
		return
	}
	_ = s.store.Session.Save(&model.Session{Last: target})
}

// persistWindow 把该窗口的工作区写入全局会话。每次关闭都会覆盖这个文件，
// 所以最后关闭的那个窗口决定了下次启动恢复什么。
func (s *AppService) persistWindow(win application.Window) {
	if win == nil {
		return
	}
	// 窗口关掉之后，还等着用户选择的拖拽导入就没有意义了。
	s.mu.Lock()
	delete(s.pendingDrops, win.ID())
	s.mu.Unlock()
	target := s.windowTarget(win.ID())
	if target == nil {
		return
	}
	if target.Kind == "directory" {
		if layout := s.captureWindowForSave(win); layout != nil {
			_ = s.store.Layouts.SaveWindow(target.Path, layout)
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
	s.mu.Lock()
	roots := make([]string, 0, len(s.roots))
	for root := range s.roots {
		roots = append(roots, root)
	}
	s.mu.Unlock()
	_ = s.store.RewritePaths(oldPath, newPath, roots)
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
	bundle, err := s.store.Settings.LoadBundle(workspacePath)
	if err != nil {
		return "", err
	}
	if workspacePath != "" {
		workspaceCandidate := filepath.Clean(filepath.Join(workspacePath, filepath.FromSlash(src)))
		return workspaceCandidate, nil
	}

	resourceDir := store.NormalizeResourceDirectory(bundle.Global.General.ResourceDirectory)
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
	globalDir, err := store.DataDir()
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
		kind := files.Classify(path, false)
		if kind != files.KindImage && kind != files.KindSVG && kind != files.KindVideo {
			http.Error(w, "unsupported", http.StatusUnsupportedMediaType)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			http.Error(w, "unreadable", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", files.MimeForExt(path))
		w.Header().Set("Cache-Control", "private, max-age=60")
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	})
}

// MediaHandler 返回挂载 /__media/ 媒体路由的中间件。
func MediaHandler(s *AppService) func(http.Handler) http.Handler {
	return s.mediaHandler
}
