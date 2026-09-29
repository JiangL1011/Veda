package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	KindMarkdown = "markdown"
	KindText     = "text"
	KindImage    = "image"
	KindSVG      = "svg"
	KindVideo    = "video"
	KindUnknown  = "unknown"
	KindDir      = "dir"
)

const maxTextBytes = 8 << 20

func shouldSkipName(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	return name == "node_modules" || name == "dist" || name == "vendor"
}

// FileEntry 是目录列表中的一行。
type FileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Kind  string `json:"kind"`
}

// FileMeta 描述磁盘上的一个路径。
type FileMeta struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Dir      string `json:"dir"`
	IsDir    bool   `json:"isDir"`
	Kind     string `json:"kind"`
	Writable bool   `json:"writable"`
	Size     int64  `json:"size"`
	Exists   bool   `json:"exists"`
}

// TextFile 是可编辑/可查看文本的数据结构。
type TextFile struct {
	FileMeta
	Content string `json:"content"`
}

func classifyFile(path string, isDir bool) string {
	if isDir {
		return KindDir
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	base := strings.ToLower(filepath.Base(path))

	switch ext {
	case "md", "markdown", "mdown", "mdx", "txt", "text":
		return KindMarkdown
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "ico", "avif", "tif", "tiff":
		return KindImage
	case "svg":
		return KindSVG
	case "mp4", "webm", "mov", "m4v", "mkv", "avi", "ogv":
		return KindVideo
	case "js", "mjs", "cjs", "ts", "tsx", "jsx", "java", "go", "xml", "html", "htm",
		"css", "scss", "less", "json", "yaml", "yml", "toml", "py", "rs", "c", "h",
		"cpp", "cc", "cxx", "hpp", "hh", "kt", "kts", "swift", "rb", "php", "sh",
		"bash", "zsh", "fish", "ps1", "sql", "vue", "svelte", "gradle", "properties",
		"ini", "conf", "cfg", "env", "cmake", "lua", "r", "dart", "cs", "vb", "pl",
		"pm", "proto", "graphql", "gql", "scala", "clj", "ex", "exs", "erl", "hs",
		"mm", "m", "asm", "s", "diff", "patch", "log", "lock", "mod", "sum",
		"gitignore", "dockerignore", "editorconfig", "dockerfile":
		return KindText
	}

	switch base {
	case "makefile", "dockerfile", "rakefile", "gemfile", "procfile", "license", "copying":
		return KindText
	}

	if ext == "" {
		return KindMarkdown
	}
	return KindUnknown
}

func sniffUnknownKind(path string, kind string) string {
	if kind != KindUnknown && kind != KindMarkdown {
		return kind
	}
	if kind == KindMarkdown && filepath.Ext(path) != "" {
		return kind
	}
	f, err := os.Open(path)
	if err != nil {
		return kind
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, _ := f.Read(buf)
	buf = buf[:n]
	if n == 0 {
		return kind
	}
	if !utf8.Valid(buf) || strings.IndexByte(string(buf), 0) >= 0 {
		if kind == KindMarkdown && filepath.Ext(path) == "" {
			return KindUnknown
		}
		return KindUnknown
	}
	if kind == KindUnknown {
		return KindText
	}
	return kind
}

func isWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func fileMeta(path string) (*FileMeta, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileMeta{Path: abs, Exists: false}, nil
		}
		return nil, err
	}
	kind := classifyFile(abs, info.IsDir())
	if !info.IsDir() {
		kind = sniffUnknownKind(abs, kind)
	}
	writable := false
	if !info.IsDir() {
		writable = isWritable(abs)
	} else {
		writable = true
	}
	return &FileMeta{
		Name:     info.Name(),
		Path:     abs,
		Dir:      filepath.Dir(abs),
		IsDir:    info.IsDir(),
		Kind:     kind,
		Writable: writable,
		Size:     info.Size(),
		Exists:   true,
	}, nil
}

func readTextFile(path string) (*TextFile, error) {
	meta, err := fileMeta(path)
	if err != nil {
		return nil, err
	}
	if !meta.Exists {
		return nil, errors.New("文件不存在")
	}
	if meta.IsDir {
		return nil, errors.New("不能读取目录")
	}
	if meta.Kind != KindMarkdown && meta.Kind != KindText {
		return nil, errors.New("不是文本文件")
	}
	if meta.Size > maxTextBytes {
		return nil, errors.New("文件过大，无法作为文本打开")
	}
	raw, err := os.ReadFile(meta.Path)
	if err != nil {
		return nil, err
	}
	if strings.IndexByte(string(raw), 0) >= 0 {
		return nil, errors.New("二进制文件无法作为文本打开")
	}
	return &TextFile{FileMeta: *meta, Content: string(raw)}, nil
}

func writeTextFile(path, content string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !isWritable(abs) {
		return errors.New("文件为只读，无法保存")
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

func sanitizeChildName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, "/")
	name = strings.TrimSuffix(name, "\\")
	if name == "" || name == "." || name == ".." {
		return "", errors.New("名称无效")
	}
	if strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return "", errors.New("名称不能包含路径分隔符")
	}
	return name, nil
}

func uniqueChildPath(parent, name string) (string, string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 0; i < 1000; i++ {
		next := name
		if i > 0 {
			next = fmt.Sprintf("%s %d%s", base, i+1, ext)
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

func createFolder(parent, name string) (*FileEntry, error) {
	abs, err := filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("只能在目录下新建")
	}
	if name == "" {
		name = "新建文件夹"
	}
	name, err = sanitizeChildName(name)
	if err != nil {
		return nil, err
	}
	full, name, err := uniqueChildPath(abs, name)
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(full, 0o755); err != nil {
		return nil, err
	}
	return &FileEntry{Name: name, Path: full, IsDir: true, Kind: KindDir}, nil
}

func createMarkdown(parent, name string) (*FileEntry, error) {
	abs, err := filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("只能在目录下新建")
	}
	if name == "" {
		name = "未命名.md"
	}
	name, err = sanitizeChildName(name)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".md" && ext != ".markdown" && ext != ".mdown" && ext != ".mdx" {
		name += ".md"
	}
	full, name, err := uniqueChildPath(abs, name)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(full, []byte(""), 0o644); err != nil {
		return nil, err
	}
	return &FileEntry{Name: name, Path: full, IsDir: false, Kind: KindMarkdown}, nil
}

func renameEntry(path, newName string) (*FileEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	newName, err = sanitizeChildName(newName)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		oldExt := filepath.Ext(filepath.Base(abs))
		if oldExt != "" && filepath.Ext(newName) == "" {
			newName += oldExt
		}
	}
	dest, err := filepath.Abs(filepath.Join(filepath.Dir(abs), newName))
	if err != nil {
		return nil, err
	}
	current := &FileEntry{
		Name:  info.Name(),
		Path:  abs,
		IsDir: info.IsDir(),
		Kind:  classifyFile(abs, info.IsDir()),
	}
	if dest == abs {
		return current, nil
	}
	if destInfo, err := os.Stat(dest); err == nil {
		if !os.SameFile(info, destInfo) {
			return nil, errors.New("已存在同名文件或文件夹")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(abs, dest); err != nil {
		return nil, err
	}
	return &FileEntry{
		Name:  newName,
		Path:  dest,
		IsDir: info.IsDir(),
		Kind:  classifyFile(dest, info.IsDir()),
	}, nil
}

func deleteEntry(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(abs)
	}
	return os.Remove(abs)
}

func readDirEntries(path string) ([]FileEntry, error) {
	return readDirEntriesWithVisiblePath(path, "")
}

func readDirEntriesWithVisiblePath(path, visiblePath string) ([]FileEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	visibleAbs := ""
	if visiblePath != "" {
		visibleAbs, _ = filepath.Abs(visiblePath)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(abs, name)
		if shouldSkipName(name) && full != visibleAbs {
			continue
		}
		info, err := e.Info()
		isDir := e.IsDir()
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			if t, lerr := os.Stat(full); lerr == nil {
				isDir = t.IsDir()
			}
		}
		out = append(out, FileEntry{
			Name:  name,
			Path:  full,
			IsDir: isDir,
			Kind:  classifyFile(full, isDir),
		})
	}
	return out, nil
}

func mimeForExt(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	case ".avif":
		return "image/avif"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".svg":
		return "image/svg+xml"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov", ".m4v":
		return "video/quicktime"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".ogv":
		return "video/ogg"
	default:
		return "application/octet-stream"
	}
}
