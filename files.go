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

// 错误码让前端按当前语言给出提示，而不必把 Go 侧的文案写死在界面里。
const (
	errCodeUnsupportedDocType = "unsupported-doc-type"
	errCodeNonTextExtension   = "non-text-extension"
	errCodeMoveIntoSelf       = "move-into-self"
)

// codedError 是带稳定错误码的错误，Error() 形如 "[code] 文案"。
// 前端解析出 code 后换成对应语言的提示，识别不了 code 时回退到原文。
type codedError struct {
	code string
	text string
}

func (e *codedError) Error() string {
	return "[" + e.code + "] " + e.text
}

func newCodedError(code, text string) error {
	return &codedError{code: code, text: text}
}

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
	// 只有 Markdown 后缀才走 Markdown 模式；.txt 是普通文本。
	case "md", "markdown", "mdown", "mdx":
		return KindMarkdown
	case "txt", "text":
		return KindText
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
		// 无后缀文件按普通文本处理，是否为文本由 sniffUnknownKind 嗅探内容决定。
		return KindText
	}
	return KindUnknown
}

// isTextExtension 判断扩展名（可带点，也可为空）是否属于可编辑的文本类型。
// 空扩展名同样允许，用于无后缀的文本文件。
func isTextExtension(ext string) bool {
	if strings.TrimPrefix(ext, ".") == "" {
		return true
	}
	switch classifyFile("file"+strings.ToLower(ext), false) {
	case KindMarkdown, KindText:
		return true
	default:
		return false
	}
}

// isTextFile 判断磁盘上的文件能否作为文本编辑，未知扩展名或无后缀时按内容嗅探。
func isTextFile(path string) bool {
	switch sniffUnknownKind(path, classifyFile(path, false)) {
	case KindMarkdown, KindText:
		return true
	default:
		return false
	}
}

// hasExtension 判断路径是否带后缀；"foo." 这类空后缀与无后缀同等看待，
// 与 classifyFile 的扩展名解析保持一致。
func hasExtension(path string) bool {
	return strings.TrimPrefix(filepath.Ext(path), ".") != ""
}

// sniffUnknownKind 在扩展名不能确定类型时嗅探文件内容：未知后缀的文本内容算普通文本，
// 无后缀文件同样按文本处理，但二进制内容会被退回未知类型。
func sniffUnknownKind(path string, kind string) string {
	unknownExtension := kind == KindUnknown
	extensionless := !hasExtension(path)
	if !unknownExtension && !(extensionless && kind == KindText) {
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
		return KindUnknown
	}
	if unknownExtension {
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
	if base == "" {
		// 形如 ".env" 的名字整体就是文件名，没有可用的主干。
		base, ext = name, ""
	}
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

// createDocument 在 parent 下创建一个不重名的文本文件。后缀由调用方决定，
// 可以为空；非文本后缀会被拒绝。
func createDocument(parent, name string) (*FileEntry, error) {
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
	if !isTextExtension(filepath.Ext(name)) {
		return nil, newCodedError(errCodeUnsupportedDocType, "只能新建文本文件")
	}
	full, name, err := uniqueChildPath(abs, name)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(full, []byte(""), 0o644); err != nil {
		return nil, err
	}
	return &FileEntry{Name: name, Path: full, IsDir: false, Kind: classifyFile(full, false)}, nil
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
		newExt := filepath.Ext(newName)
		if !strings.EqualFold(newExt, oldExt) {
			text := isTextFile(abs)
			// 没填写后缀时保留原后缀：文本文件允许去掉后缀，其他文件等于只改文件名。
			if newExt == "" && oldExt != "" && !text {
				newName += oldExt
				newExt = oldExt
			}
			if !strings.EqualFold(newExt, oldExt) {
				if !text {
					return nil, newCodedError(errCodeNonTextExtension, "非文本文件不能修改后缀")
				}
				if !isTextExtension(newExt) {
					return nil, newCodedError(errCodeUnsupportedDocType, "只能修改为文本文件后缀")
				}
			}
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

// moveEntryTo 把 path 移动到 destDir 里，返回移动后的条目。目标目录已有同名项时
// 追加 (n) 后缀，不会覆盖任何现有内容。
func moveEntryTo(path, destDir string) (*FileEntry, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	destInfo, err := os.Stat(destAbs)
	if err != nil {
		return nil, err
	}
	if !destInfo.IsDir() {
		return nil, errors.New("目标不是文件夹")
	}
	// 把文件夹搬进它自己或它的子目录会无限套娃。
	if info.IsDir() && isSameOrInside(abs, destAbs) {
		return nil, newCodedError(errCodeMoveIntoSelf, "不能把文件夹移动到它自己或子目录里")
	}
	current := &FileEntry{
		Name:  info.Name(),
		Path:  abs,
		IsDir: info.IsDir(),
		Kind:  classifyFile(abs, info.IsDir()),
	}
	if filepath.Dir(abs) == destAbs {
		return current, nil
	}
	dest, name, err := uniqueChildPathParen(destAbs, filepath.Base(abs))
	if err != nil {
		return nil, err
	}
	if err := transferPath(abs, dest, true); err != nil {
		return nil, err
	}
	return &FileEntry{
		Name:  name,
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
