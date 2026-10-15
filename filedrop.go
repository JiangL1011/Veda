package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileDropOutcome 是一次拖拽导入的结果：落盘的文件，以及失败项的原因。
type FileDropOutcome struct {
	DestDir   string      `json:"destDir"`
	Workspace string      `json:"workspace"`
	Action    string      `json:"action"`
	Imported  []FileEntry `json:"imported"`
	Errors    []string    `json:"errors"`
}

// uniqueChildPathParen 为 parent 下的 name 生成一个不重名的路径：重名时在主干后
// 追加 (1)、(2)……后缀原样保留，所以 "note.md" 会变成 "note(1).md"。
func uniqueChildPathParen(parent, name string) (string, string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		// 形如 ".env" 的名字整体就是文件名，没有可用的主干。
		base, ext = name, ""
	}
	for i := 0; i < 1000; i++ {
		next := name
		if i > 0 {
			next = fmt.Sprintf("%s(%d)%s", base, i, ext)
		}
		full := filepath.Join(parent, next)
		if _, err := os.Stat(full); err != nil {
			if os.IsNotExist(err) {
				return full, next, nil
			}
			return "", "", err
		}
	}
	return "", "", errors.New("无法生成唯一名称")
}

// importPathsInto 依次把 files 复制（move 为 false）或移动到 destDir。
// 单个文件失败不会中断其余文件，失败原因按 "文件名: 原因" 收集到 Errors 里。
func importPathsInto(files []string, destDir, workspace string, move bool) *FileDropOutcome {
	action := dragDropCopy
	if move {
		action = dragDropMove
	}
	out := &FileDropOutcome{
		DestDir:   destDir,
		Workspace: workspace,
		Action:    action,
		Imported:  []FileEntry{},
		Errors:    []string{},
	}
	for _, src := range files {
		entry, err := importPathInto(src, destDir, move)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("%s：%v", filepath.Base(src), err))
			continue
		}
		out.Imported = append(out.Imported, entry)
	}
	return out
}

// importPathInto 把一个文件或文件夹放进 destDir，返回落盘后的条目。
func importPathInto(src, destDir string, move bool) (FileEntry, error) {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return FileEntry{}, err
	}
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return FileEntry{}, err
	}
	info, err := os.Stat(absSrc)
	if err != nil {
		return FileEntry{}, err
	}
	destInfo, err := os.Stat(absDest)
	if err != nil {
		return FileEntry{}, err
	}
	if !destInfo.IsDir() {
		return FileEntry{}, errors.New("目标不是文件夹")
	}
	if info.IsDir() && isSameOrInside(absSrc, absDest) {
		// 否则会无限套娃，或者把文件夹搬进它自己。
		return FileEntry{}, errors.New("不能把文件夹放进它自己的子目录")
	}
	dest, name, err := uniqueChildPathParen(absDest, filepath.Base(absSrc))
	if err != nil {
		return FileEntry{}, err
	}
	if err := transferPath(absSrc, dest, move); err != nil {
		return FileEntry{}, err
	}
	return FileEntry{
		Name:  name,
		Path:  dest,
		IsDir: info.IsDir(),
		Kind:  classifyFile(dest, info.IsDir()),
	}, nil
}

// isSameOrInside 判断 path 是否就是 parent 或位于 parent 内部。
func isSameOrInside(parent, path string) bool {
	if parent == path {
		return true
	}
	return strings.HasPrefix(path, parent+string(os.PathSeparator))
}

// transferPath 把 src 送到 dest。移动优先用重命名（同一分区内是原子操作），
// 跨分区失败时退回复制后删除源。
func transferPath(src, dest string, move bool) error {
	if move {
		if err := os.Rename(src, dest); err == nil {
			return nil
		}
		if err := copyPath(src, dest); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}
	return copyPath(src, dest)
}

func copyPath(src, dest string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dest)
	}
	if info.IsDir() {
		return copyDir(src, dest, info)
	}
	return copyFileContents(src, dest, info.Mode().Perm())
}

func copyDir(src, dest string, info os.FileInfo) error {
	if err := os.Mkdir(dest, info.Mode().Perm()|0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyPath(filepath.Join(src, entry.Name()), filepath.Join(dest, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFileContents(src, dest string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return err
	}
	return out.Close()
}
