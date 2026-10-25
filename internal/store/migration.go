package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"veda/internal/model"
)

type dataMigrationTracker struct {
	mu          sync.Mutex
	started     bool
	ready       chan struct{}
	readyClosed bool
	state       model.DataMigrationState
}

var dataMigration = &dataMigrationTracker{ready: make(chan struct{})}

// StartDataMigration 在应用启动时调用一次：索引已存在就什么都不做，
// 不存在（或已损坏）就在后台重建并改写各个 JSON 配置。
func StartDataMigration() {
	dataMigration.mu.Lock()
	if dataMigration.started {
		dataMigration.mu.Unlock()
		return
	}
	dataMigration.started = true
	dataMigration.mu.Unlock()

	_, exists, err := readFileIndex()
	if err == nil && exists {
		dataMigration.mu.Lock()
		dataMigration.state = model.DataMigrationState{Done: true}
		dataMigration.closeReadyLocked()
		dataMigration.mu.Unlock()
		return
	}

	dataMigration.mu.Lock()
	dataMigration.state = model.DataMigrationState{Required: true, Running: true}
	dataMigration.mu.Unlock()
	go runDataMigration()
}

func (t *dataMigrationTracker) closeReadyLocked() {
	if !t.readyClosed {
		t.readyClosed = true
		close(t.ready)
	}
}

// CurrentMigrationState 返回当前迁移状态，供前端轮询。
func CurrentMigrationState() model.DataMigrationState {
	dataMigration.mu.Lock()
	defer dataMigration.mu.Unlock()
	if !dataMigration.started {
		return model.DataMigrationState{Done: true}
	}
	return dataMigration.state
}

// MigrationInProgress 报告迁移是否仍在进行中。
func MigrationInProgress() bool {
	return CurrentMigrationState().Running
}

// waitForDataMigration 让持久化操作等到迁移结束。没有发起迁移时立即返回。
func waitForDataMigration() {
	dataMigration.mu.Lock()
	if !dataMigration.started || dataMigration.state.Done || dataMigration.state.Error != "" {
		dataMigration.mu.Unlock()
		return
	}
	ready := dataMigration.ready
	dataMigration.mu.Unlock()
	<-ready
}

func runDataMigration() {
	processed, total := 0, 0
	err := migrateFileIndex(func(done, count int) {
		processed, total = done, count
		dataMigration.mu.Lock()
		dataMigration.state.Processed = done
		dataMigration.state.Total = count
		dataMigration.mu.Unlock()
	})

	dataMigration.mu.Lock()
	defer dataMigration.mu.Unlock()
	dataMigration.state.Running = false
	dataMigration.state.Processed = processed
	dataMigration.state.Total = total
	if err != nil {
		dataMigration.state.Error = err.Error()
	} else {
		dataMigration.state.Done = true
	}
	dataMigration.closeReadyLocked()
}

// RetryDataMigration 在迁移失败后由界面触发重试。
func RetryDataMigration() model.DataMigrationState {
	dataMigration.mu.Lock()
	if dataMigration.state.Running {
		state := dataMigration.state
		dataMigration.mu.Unlock()
		return state
	}
	dataMigration.started = true
	dataMigration.ready = make(chan struct{})
	dataMigration.readyClosed = false
	dataMigration.state = model.DataMigrationState{Required: true, Running: true}
	dataMigration.mu.Unlock()
	go runDataMigration()
	return CurrentMigrationState()
}

type fileIndexBuilder struct {
	files map[string]string
	used  map[string]struct{}
}

func newFileIndexBuilder() *fileIndexBuilder {
	return &fileIndexBuilder{
		files: map[string]string{},
		used:  map[string]struct{}{},
	}
}

// seed 把上一轮已经写出的索引带进来。迁移可能在中途失败，重试时
// 必须沿用已经分配到配置文件里的编号，否则旧编号会变成悬空引用。
func (b *fileIndexBuilder) seed(files map[string]string) {
	for path, id := range files {
		if id == "" {
			continue
		}
		key, err := normalizeFilePath(path)
		if err != nil {
			continue
		}
		if _, exists := b.used[id]; exists {
			continue
		}
		b.files[key] = id
		b.used[id] = struct{}{}
	}
}

func (b *fileIndexBuilder) idFor(path string) (string, error) {
	key, err := normalizeFilePath(path)
	if err != nil {
		return "", err
	}
	if id := b.files[key]; id != "" {
		return id, nil
	}
	id, err := allocateFileID(b.used)
	if err != nil {
		return "", err
	}
	b.files[key] = id
	return id, nil
}

type dataMigrationPlan struct {
	path string
	doc  any
}

// migrateFileIndex 扫描整个 ~/.veda，收集所有被记录的文件路径，建立索引，
// 再把各 JSON 配置里的路径改写成索引编号。
func migrateFileIndex(onProgress func(processed, total int)) error {
	root, err := dataDir()
	if err != nil {
		return err
	}

	var jsonFiles []string
	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			return nil
		}
		if entry.Name() == fileIndexFileName {
			return nil
		}
		jsonFiles = append(jsonFiles, path)
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	sort.Strings(jsonFiles)

	total := len(jsonFiles)
	if onProgress != nil {
		onProgress(0, total)
	}

	workspaceIdx, workspaceLoaded := loadMigrationWorkspaceIndex(root)
	builder := newFileIndexBuilder()
	if existing, _, indexErr := readFileIndex(); indexErr == nil {
		builder.seed(existing)
	}
	for path := range workspaceIdx.Workspaces {
		if path == "" {
			continue
		}
		if _, err := builder.idFor(path); err != nil {
			return err
		}
	}

	workspaceByID := make(map[string]string, len(workspaceIdx.Workspaces))
	for path, id := range workspaceIdx.Workspaces {
		if id != "" {
			workspaceByID[id] = path
		}
	}

	plans := make([]dataMigrationPlan, 0, len(jsonFiles))
	workspaceChanged := false
	for i, jsonPath := range jsonFiles {
		doc, changed, wsChanged, err := planDataMigration(jsonPath, root, workspaceIdx, workspaceByID, builder, workspaceLoaded)
		if err == nil && changed {
			plans = append(plans, dataMigrationPlan{path: jsonPath, doc: doc})
		}
		if wsChanged {
			workspaceChanged = true
		}
		if onProgress != nil {
			onProgress(i+1, total)
		}
	}

	if workspaceChanged && workspaceLoaded {
		if err := saveMigrationWorkspaceIndex(root, workspaceIdx); err != nil {
			return err
		}
	}
	// 先落盘索引。后面即使中途失败，已经写好的索引仍然可用。
	if err := writeFileIndex(builder.files); err != nil {
		return err
	}
	for _, plan := range plans {
		if err := writeJSONFile(plan.path, plan.doc); err != nil {
			return err
		}
	}
	return nil
}

func loadMigrationWorkspaceIndex(root string) (*workspaceIndex, bool) {
	idx := &workspaceIndex{Workspaces: map[string]string{}}
	raw, err := os.ReadFile(filepath.Join(root, workspaceMapFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return idx, true
		}
		return idx, false
	}
	if len(bytesTrimSpace(raw)) == 0 {
		return idx, true
	}
	if err := json.Unmarshal(raw, idx); err != nil {
		return idx, false
	}
	if idx.Workspaces == nil {
		idx.Workspaces = map[string]string{}
	}
	return idx, true
}

func saveMigrationWorkspaceIndex(root string, idx *workspaceIndex) error {
	if idx.Workspaces == nil {
		idx.Workspaces = map[string]string{}
	}
	return writeJSONFile(filepath.Join(root, workspaceMapFileName), idx)
}

func ensureMigrationWorkspaceID(idx *workspaceIndex, absPath string) (string, error) {
	if id := idx.Workspaces[absPath]; id != "" {
		return id, nil
	}
	used := usedWorkspaceIDs(idx)
	for i := 0; i < 8; i++ {
		id, err := newWorkspaceID()
		if err != nil {
			return "", err
		}
		if _, exists := used[id]; exists {
			continue
		}
		idx.Workspaces[absPath] = id
		return id, nil
	}
	return "", errors.New("无法分配工作区编号")
}

func planDataMigration(
	jsonPath, root string,
	workspaceIdx *workspaceIndex,
	workspaceByID map[string]string,
	builder *fileIndexBuilder,
	workspaceLoaded bool,
) (any, bool, bool, error) {
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, false, false, err
	}
	if len(bytesTrimSpace(raw)) == 0 {
		return nil, false, false, nil
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, false, err
	}

	switch filepath.Base(jsonPath) {
	case workspaceMapFileName:
		// workspace.json 保留“工作区路径 → 工作区编号”的结构，不在这里改写。
		return doc, false, false, nil
	case sessionFileName:
		changed, workspaceChanged, err := migrateSessionDoc(doc, workspaceIdx, builder, workspaceLoaded)
		return doc, changed, workspaceChanged, err
	case layoutFileName:
		changed, err := migrateLayoutDoc(doc, builder)
		return doc, changed, false, err
	case locksFileName:
		workspace := workspacePathForLocks(jsonPath, root, workspaceByID)
		changed, err := migrateLocksDoc(doc, workspace, builder)
		return doc, changed, false, err
	default:
		return doc, false, false, nil
	}
}

func migrationIDForPath(builder *fileIndexBuilder, value string) (string, bool, error) {
	if value == "" || isUUID(value) {
		return value, false, nil
	}
	if !filepath.IsAbs(value) {
		return value, false, nil
	}
	id, err := builder.idFor(value)
	if err != nil {
		return value, false, err
	}
	return id, id != value, nil
}

func migrateSessionDoc(
	doc any,
	workspaceIdx *workspaceIndex,
	builder *fileIndexBuilder,
	workspaceLoaded bool,
) (bool, bool, error) {
	m, ok := doc.(map[string]any)
	if !ok {
		return false, false, nil
	}
	last, ok := m["last"].(map[string]any)
	if !ok {
		return false, false, nil
	}
	kind, _ := last["kind"].(string)
	changed := false
	workspaceChanged := false

	pathValue, _ := last["path"].(string)
	if kind == "directory" {
		if pathValue != "" && !isUUID(pathValue) {
			abs, err := normalizeFilePath(pathValue)
			if err == nil && workspaceLoaded {
				id, idErr := ensureMigrationWorkspaceID(workspaceIdx, abs)
				if idErr == nil {
					last["path"] = id
					changed = true
					workspaceChanged = true
					if _, buildErr := builder.idFor(abs); buildErr != nil {
						return changed, workspaceChanged, buildErr
					}
				}
			}
		}
	} else {
		id, idChanged, err := migrationIDForPath(builder, pathValue)
		if err != nil {
			return changed, workspaceChanged, err
		}
		if idChanged {
			last["path"] = id
			changed = true
		}
	}

	if fileValue, ok := last["file"].(string); ok && fileValue != "" {
		id, idChanged, err := migrationIDForPath(builder, fileValue)
		if err != nil {
			return changed, workspaceChanged, err
		}
		if idChanged {
			last["file"] = id
			changed = true
		}
	}
	return changed, workspaceChanged, nil
}

func migrateLayoutDoc(doc any, builder *fileIndexBuilder) (bool, error) {
	m, ok := doc.(map[string]any)
	if !ok {
		return false, nil
	}
	changed := false
	if tabs, ok := m["openTabs"].([]any); ok {
		next := make([]any, 0, len(tabs))
		for _, item := range tabs {
			value, isString := item.(string)
			if !isString {
				next = append(next, item)
				continue
			}
			id, idChanged, err := migrationIDForPath(builder, value)
			if err != nil {
				return false, err
			}
			if idChanged {
				changed = true
			}
			next = append(next, id)
		}
		m["openTabs"] = next
	}
	if active, ok := m["activeTab"].(string); ok && active != "" {
		id, idChanged, err := migrationIDForPath(builder, active)
		if err != nil {
			return false, err
		}
		if idChanged {
			m["activeTab"] = id
			changed = true
		}
	}
	return changed, nil
}

func workspacePathForLocks(jsonPath, root string, workspaceByID map[string]string) string {
	dir := filepath.Clean(filepath.Dir(jsonPath))
	if dir == filepath.Clean(root) {
		return ""
	}
	return workspaceByID[filepath.Base(dir)]
}

func migrateLocksDoc(doc any, workspacePath string, builder *fileIndexBuilder) (bool, error) {
	m, ok := doc.(map[string]any)
	if !ok {
		return false, nil
	}
	files, ok := m["files"].(map[string]any)
	if !ok || len(files) == 0 {
		return false, nil
	}
	next := make(map[string]any, len(files))
	changed := false
	for key, mode := range files {
		if key == "" || isUUID(key) {
			next[key] = mode
			continue
		}
		path := key
		if !filepath.IsAbs(path) {
			if workspacePath == "" {
				next[key] = mode
				continue
			}
			path = filepath.Join(workspacePath, filepath.FromSlash(path))
		}
		id, err := builder.idFor(path)
		if err != nil {
			return false, err
		}
		next[id] = mode
		changed = true
	}
	if !changed {
		return false, nil
	}
	m["files"] = next
	return true, nil
}
