package store

import (
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
	return newUUID()
}

func usedWorkspaceIDs(idx *workspaceIndex) map[string]struct{} {
	used := make(map[string]struct{}, len(idx.Workspaces))
	for _, id := range idx.Workspaces {
		used[id] = struct{}{}
	}
	return used
}

func ensureWorkspaceID(abs string) (string, error) {
	workspaceMu.Lock()
	defer workspaceMu.Unlock()
	return ensureWorkspaceIDLocked(abs)
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
	// 工作区目录本身也登记到全局文件索引里，这样重命名/删除工作区时
	// 文件索引和目录树记录的是同一个身份。
	_, _ = EnsureFileID(abs)
	return id, nil
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

// workspacePathByID 把 workspace.json 里的工作区编号还原成路径。
func workspacePathByID(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	workspaceMu.Lock()
	defer workspaceMu.Unlock()
	idx, err := loadWorkspaceIndexLocked()
	if err != nil {
		return "", false
	}
	for path, workspaceID := range idx.Workspaces {
		if workspaceID == id {
			return path, true
		}
	}
	return "", false
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
