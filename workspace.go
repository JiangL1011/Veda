package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const workspaceMapFileName = "workspace.json"

type workspaceIndex struct {
	Workspaces map[string]string `json:"workspaces"`
}

var workspaceMu sync.Mutex

func absWorkspaceDir(workspacePath string) (string, error) {
	if strings.TrimSpace(workspacePath) == "" {
		return "", errors.New("工作区路径必须是文件夹")
	}
	abs, err := filepath.Abs(workspacePath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("工作区路径必须是文件夹")
	}
	return abs, nil
}

func workspaceMapPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, workspaceMapFileName), nil
}

func loadWorkspaceIndexLocked() (*workspaceIndex, error) {
	path, err := workspaceMapPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &workspaceIndex{Workspaces: map[string]string{}}, nil
		}
		return nil, err
	}
	var idx workspaceIndex
	if len(bytesTrimSpace(raw)) == 0 {
		return &workspaceIndex{Workspaces: map[string]string{}}, nil
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, err
	}
	if idx.Workspaces == nil {
		idx.Workspaces = map[string]string{}
	}
	return &idx, nil
}

func saveWorkspaceIndexLocked(idx *workspaceIndex) error {
	if idx.Workspaces == nil {
		idx.Workspaces = map[string]string{}
	}
	path, err := workspaceMapPath()
	if err != nil {
		return err
	}
	return writeJSONFile(path, idx)
}

func newWorkspaceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32], nil
}

func usedWorkspaceIDs(idx *workspaceIndex) map[string]struct{} {
	used := make(map[string]struct{}, len(idx.Workspaces))
	for _, id := range idx.Workspaces {
		used[id] = struct{}{}
	}
	return used
}

func ensureWorkspaceIDLocked(abs string) (string, error) {
	idx, err := loadWorkspaceIndexLocked()
	if err != nil {
		return "", err
	}
	if id := idx.Workspaces[abs]; id != "" {
		return id, nil
	}
	used := usedWorkspaceIDs(idx)
	var id string
	for i := 0; i < 8; i++ {
		id, err = newWorkspaceID()
		if err != nil {
			return "", err
		}
		if _, exists := used[id]; !exists {
			break
		}
		id = ""
	}
	if id == "" {
		return "", errors.New("无法分配工作区编号")
	}
	idx.Workspaces[abs] = id
	if err := saveWorkspaceIndexLocked(idx); err != nil {
		return "", err
	}
	return id, nil
}

// mergeLegacyDir 把 legacy 目录中尚未存在于 dest 的条目移动过去，然后删除 legacy。
func mergeLegacyDir(legacy, dest string) error {
	info, err := os.Stat(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		src := filepath.Join(legacy, entry.Name())
		dst := filepath.Join(dest, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return os.RemoveAll(legacy)
}

// migrateLegacyDataDir 把旧版本 Marknote 的 ~/.marknote 迁移到 ~/.veda。
func migrateLegacyDataDir(home, dest string) error {
	return mergeLegacyDir(filepath.Join(home, legacyDataDirName), dest)
}

// migrateLegacyWorkspaceDir 把工作区里遗留的 .veda/.marknote 目录迁移到全局数据目录。
func migrateLegacyWorkspaceDir(abs, dest string) error {
	for _, name := range []string{dataDirName, legacyDataDirName} {
		if err := mergeLegacyDir(filepath.Join(abs, name), dest); err != nil {
			return err
		}
	}
	return nil
}

func workspaceDataDir(workspacePath string) (string, error) {
	workspaceMu.Lock()
	defer workspaceMu.Unlock()
	return workspaceDataDirLocked(workspacePath)
}

func workspaceDataDirLocked(workspacePath string) (string, error) {
	abs, err := absWorkspaceDir(workspacePath)
	if err != nil {
		return "", err
	}
	id, err := ensureWorkspaceIDLocked(abs)
	if err != nil {
		return "", err
	}
	root, err := dataDir()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(root, id)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return "", err
	}
	if err := migrateLegacyWorkspaceDir(abs, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func workspaceFile(workspacePath, name string) (string, error) {
	dir, err := workspaceDataDir(workspacePath)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

type workspaceStore struct {
	Path string
	Dir  string
}

func listedWorkspaceStores() []workspaceStore {
	workspaceMu.Lock()
	defer workspaceMu.Unlock()

	idx, err := loadWorkspaceIndexLocked()
	if err != nil {
		return nil
	}
	root, err := dataDir()
	if err != nil {
		return nil
	}
	out := make([]workspaceStore, 0, len(idx.Workspaces))
	for path, id := range idx.Workspaces {
		if path == "" || id == "" {
			continue
		}
		out = append(out, workspaceStore{
			Path: path,
			Dir:  filepath.Join(root, id),
		})
	}
	return out
}
