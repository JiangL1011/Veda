package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const locksFileName = "locks.json"

const (
	lockModeEdit     = "edit"
	lockModeReadonly = "readonly"
)

// FileLockState 是文档持久化的锁定状态（只读还是可编辑）。
type FileLockState struct {
	Found   bool `json:"found"`
	Editing bool `json:"editing"`
}

type fileLocksDoc struct {
	Files map[string]string `json:"files"`
}

type lockStore struct {
	mu sync.Mutex
}

func locksPath(workspacePath string) (string, error) {
	if workspacePath == "" {
		dir, err := dataDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, locksFileName), nil
	}
	return workspaceFile(workspacePath, locksFileName)
}

func lockKey(workspacePath, filePath string) (string, error) {
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}
	if workspacePath == "" {
		return filepath.ToSlash(absFile), nil
	}
	absWS, err := filepath.Abs(workspacePath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absWS, absFile)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return filepath.ToSlash(absFile), nil
	}
	return filepath.ToSlash(rel), nil
}

func readLocksFile(path string) (fileLocksDoc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileLocksDoc{Files: map[string]string{}}, nil
		}
		return fileLocksDoc{}, err
	}
	var doc fileLocksDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fileLocksDoc{}, err
	}
	if doc.Files == nil {
		doc.Files = map[string]string{}
	}
	return doc, nil
}

func parseLockMode(value string) (editing bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case lockModeEdit:
		return true, true
	case lockModeReadonly:
		return false, true
	default:
		return false, false
	}
}

func rewriteLockKeys(files map[string]string, oldKey, newKey string) (map[string]string, bool) {
	if files == nil || oldKey == "" || oldKey == newKey {
		return files, false
	}
	changed := false
	next := make(map[string]string, len(files))
	for key, mode := range files {
		rewritten, ok := rewriteConfigPath(key, oldKey, newKey)
		if ok {
			changed = true
			next[rewritten] = mode
			continue
		}
		next[key] = mode
	}
	if !changed {
		return files, false
	}
	return next, true
}

func (s *lockStore) get(workspacePath, filePath string) (FileLockState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fallback := FileLockState{Editing: true}
	waitForDataMigration()
	path, err := locksPath(workspacePath)
	if err != nil {
		return fallback, err
	}
	doc, err := readLocksFile(path)
	if err != nil {
		return fallback, err
	}
	var mode string
	exists := false
	if id, ok := fileIDForPath(filePath); ok {
		mode, exists = doc.Files[id]
	}
	if !exists {
		// 兼容还没迁移过的旧键（工作区相对路径或绝对路径）。
		if legacy, legacyErr := lockKey(workspacePath, filePath); legacyErr == nil {
			mode, exists = doc.Files[legacy]
		}
	}
	if !exists {
		return fallback, nil
	}
	editing, ok := parseLockMode(mode)
	if !ok {
		return fallback, nil
	}
	return FileLockState{Found: true, Editing: editing}, nil
}

func (s *lockStore) save(workspacePath, filePath string, editing bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	waitForDataMigration()
	id, err := ensureFileID(filePath)
	if err != nil {
		return err
	}
	path, err := locksPath(workspacePath)
	if err != nil {
		return err
	}
	doc, err := readLocksFile(path)
	if err != nil {
		return err
	}
	if doc.Files == nil {
		doc.Files = map[string]string{}
	}
	if editing {
		doc.Files[id] = lockModeEdit
	} else {
		doc.Files[id] = lockModeReadonly
	}
	return writeJSONFile(path, doc)
}

// rewrite 让锁定状态跟着文件重命名/移动。locks.json 里存的是文件编号，
// 编号跟着 file-index.json 一起迁移，因此这里只需要移动索引。
func (s *lockStore) rewrite(workspacePath, oldPath, newPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return moveFileIndex(oldPath, newPath)
}
