package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// fileIndexFileName 是全局唯一的文件索引文件，位于 ~/.veda/file-index.json。
// 它的内容是一个简单的对象：键为文件的全路径，值为分配给该文件的 UUID。
const fileIndexFileName = "file-index.json"

// fileIndexMu 串行化对所有 file-index.json 的读改写。
var fileIndexMu sync.Mutex

// normalizeFilePath 把路径收敛成绝对、清理过的形式，作为索引的键。
func normalizeFilePath(path string) (string, error) {
	if path == "" {
		return "", errors.New("空路径")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func fileIndexPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileIndexFileName), nil
}

// readFileIndex 读取索引。第二个返回值表示索引文件是否已经存在。
func readFileIndex() (map[string]string, bool, error) {
	path, err := fileIndexPath()
	if err != nil {
		return nil, false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, false, nil
		}
		return nil, false, err
	}
	files := map[string]string{}
	if len(bytesTrimSpace(raw)) == 0 {
		return files, true, nil
	}
	if err := json.Unmarshal(raw, &files); err != nil {
		return nil, true, err
	}
	if files == nil {
		files = map[string]string{}
	}
	return files, true, nil
}

func writeFileIndex(files map[string]string) error {
	if files == nil {
		files = map[string]string{}
	}
	path, err := fileIndexPath()
	if err != nil {
		return err
	}
	return writeJSONFile(path, files)
}

func usedFileIDs(files map[string]string) map[string]struct{} {
	used := make(map[string]struct{}, len(files))
	for _, id := range files {
		if id != "" {
			used[id] = struct{}{}
		}
	}
	return used
}

func allocateFileID(used map[string]struct{}) (string, error) {
	for i := 0; i < 8; i++ {
		id, err := newUUID()
		if err != nil {
			return "", err
		}
		if _, exists := used[id]; !exists {
			used[id] = struct{}{}
			return id, nil
		}
	}
	return "", errors.New("无法分配文件编号")
}

// EnsureFileIDs 返回这些路径对应的编号，并为还没有编号的路径分配一个。
func EnsureFileIDs(paths []string) (map[string]string, error) {
	out := make(map[string]string, len(paths))
	keys := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		key, err := normalizeFilePath(path)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return out, nil
	}

	waitForDataMigration()
	fileIndexMu.Lock()
	defer fileIndexMu.Unlock()

	files, _, err := readFileIndex()
	if err != nil {
		return nil, err
	}
	used := usedFileIDs(files)
	changed := false
	for _, key := range keys {
		id := files[key]
		if id == "" {
			id, err = allocateFileID(used)
			if err != nil {
				return nil, err
			}
			files[key] = id
			changed = true
		}
		out[key] = id
	}
	if changed {
		if err := writeFileIndex(files); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func EnsureFileID(path string) (string, error) {
	ids, err := EnsureFileIDs([]string{path})
	if err != nil {
		return "", err
	}
	key, err := normalizeFilePath(path)
	if err != nil {
		return "", err
	}
	return ids[key], nil
}

// fileIDForPath 只查询已有编号，不会为路径新建索引项。
func fileIDForPath(path string) (string, bool) {
	key, err := normalizeFilePath(path)
	if err != nil {
		return "", false
	}
	waitForDataMigration()
	fileIndexMu.Lock()
	defer fileIndexMu.Unlock()
	files, _, err := readFileIndex()
	if err != nil {
		return "", false
	}
	id := files[key]
	return id, id != ""
}

// pathForFileID 把编号还原成文件全路径。
func pathForFileID(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	waitForDataMigration()
	fileIndexMu.Lock()
	defer fileIndexMu.Unlock()
	files, _, err := readFileIndex()
	if err != nil {
		return "", false
	}
	for path, fileID := range files {
		if fileID == id {
			return path, true
		}
	}
	return "", false
}

// storedFilePath 兼容两种存储形式：新版本存编号，旧版本直接存路径。
func storedFilePath(value string) string {
	return storedFilePaths([]string{value})[0]
}

func storedFilePaths(values []string) []string {
	out := make([]string, len(values))
	if len(values) == 0 {
		return out
	}
	ids := make([]string, 0, len(values))
	idIndexes := make([]int, 0, len(values))
	waitForDataMigration()
	fileIndexMu.Lock()
	files, _, err := readFileIndex()
	byID := map[string]string{}
	if err == nil {
		byID = make(map[string]string, len(files))
		for path, id := range files {
			byID[id] = path
		}
	}
	fileIndexMu.Unlock()
	for i, value := range values {
		out[i] = value
		if value == "" || !isUUID(value) {
			continue // 旧数据里的真实路径，原样保留
		}
		ids = append(ids, value)
		idIndexes = append(idIndexes, i)
	}
	for i, id := range ids {
		out[idIndexes[i]] = byID[id]
	}
	return out
}

// RemoveFileIndex 删除某个路径及其所有子项的索引。
func RemoveFileIndex(path string) error {
	key, err := normalizeFilePath(path)
	if err != nil {
		return err
	}
	waitForDataMigration()
	fileIndexMu.Lock()
	defer fileIndexMu.Unlock()
	files, _, err := readFileIndex()
	if err != nil {
		return err
	}
	prefix := key + string(os.PathSeparator)
	changed := false
	for current := range files {
		if current == key || strings.HasPrefix(current, prefix) {
			delete(files, current)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return writeFileIndex(files)
}

// MoveFileIndex 把 oldPath（包括其子项）的编号迁移到 newPath。
func MoveFileIndex(oldPath, newPath string) error {
	oldKey, err := normalizeFilePath(oldPath)
	if err != nil {
		return err
	}
	newKey, err := normalizeFilePath(newPath)
	if err != nil {
		return err
	}
	if oldKey == newKey {
		return nil
	}
	waitForDataMigration()
	fileIndexMu.Lock()
	defer fileIndexMu.Unlock()
	files, _, err := readFileIndex()
	if err != nil {
		return err
	}
	prefix := oldKey + string(os.PathSeparator)
	moved := make(map[string]string)
	changed := false
	for current, id := range files {
		if current == oldKey {
			moved[newKey] = id
			delete(files, current)
			changed = true
			continue
		}
		if strings.HasPrefix(current, prefix) {
			moved[newKey+current[len(oldKey):]] = id
			delete(files, current)
			changed = true
		}
	}
	for path, id := range moved {
		files[path] = id
	}
	if !changed {
		return nil
	}
	return writeFileIndex(files)
}

// collectTreePaths 列出 root 及其所有子项（包括目录本身）。
func collectTreePaths(root string) ([]string, error) {
	abs, err := normalizeFilePath(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(abs); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	paths := make([]string, 0, 16)
	err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// 没有权限的条目跳过，其余文件照常登记。
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// EnsureFileTree 把磁盘上的整个目录树登记进索引。
func EnsureFileTree(root string) error {
	paths, err := collectTreePaths(root)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	_, err = EnsureFileIDs(paths)
	return err
}

// TransferFileIndex 同步拖拽导入后的索引：复制为新文件分配新编号，
// 移动则尽量把原文件的编号带到新位置。
func TransferFileIndex(src, dst string, removeSource bool) error {
	destPaths, err := collectTreePaths(dst)
	if err != nil {
		return err
	}
	srcKey, err := normalizeFilePath(src)
	if err != nil {
		return err
	}
	dstKey, err := normalizeFilePath(dst)
	if err != nil {
		return err
	}

	waitForDataMigration()
	fileIndexMu.Lock()
	defer fileIndexMu.Unlock()

	files, _, err := readFileIndex()
	if err != nil {
		return err
	}
	changed := false
	if removeSource && srcKey != dstKey {
		prefix := srcKey + string(os.PathSeparator)
		moved := make(map[string]string)
		for current, id := range files {
			if current == srcKey {
				moved[dstKey] = id
				delete(files, current)
				changed = true
				continue
			}
			if strings.HasPrefix(current, prefix) {
				moved[dstKey+current[len(srcKey):]] = id
				delete(files, current)
				changed = true
			}
		}
		for path, id := range moved {
			files[path] = id
		}
	}
	used := usedFileIDs(files)
	for _, path := range destPaths {
		if files[path] != "" {
			continue
		}
		id, allocErr := allocateFileID(used)
		if allocErr != nil {
			return allocErr
		}
		files[path] = id
		changed = true
	}
	if !changed {
		return nil
	}
	return writeFileIndex(files)
}
