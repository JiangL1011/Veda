package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// previewPrefix 是 HTML 预览使用的资源地址前缀。
//
// 地址形如 /__preview/<根目录>/<相对路径>：根目录以 base64 编码，后面的
// 相对路径保持目录结构。这样 HTML 里的相对引用（样式、脚本、图片）会被
// 浏览器解析成同一前缀下的地址，从而能正常加载。
const previewPrefix = "/__preview/"

// previewAssetTypes 是预览页面允许加载的资源类型白名单。
// 只有这里的扩展名会被预览路由提供，避免把工作区里的任意文件暴露给页面。
var previewAssetTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".htm":         "text/html; charset=utf-8",
	".xhtml":       "application/xhtml+xml; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".cjs":         "text/javascript; charset=utf-8",
	".json":        "application/json; charset=utf-8",
	".map":         "application/json; charset=utf-8",
	".webmanifest": "application/manifest+json; charset=utf-8",
	".txt":         "text/plain; charset=utf-8",
	".csv":         "text/csv; charset=utf-8",
	".xml":         "application/xml; charset=utf-8",
	".wasm":        "application/wasm",
	".woff":        "font/woff",
	".woff2":       "font/woff2",
	".ttf":         "font/ttf",
	".otf":         "font/otf",
	".eot":         "application/vnd.ms-fontobject",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".bmp":         "image/bmp",
	".ico":         "image/x-icon",
	".tif":         "image/tiff",
	".tiff":        "image/tiff",
	".svg":         "image/svg+xml",
	".mp4":         "video/mp4",
	".webm":        "video/webm",
	".mov":         "video/quicktime",
	".m4v":         "video/quicktime",
	".ogv":         "video/ogg",
	".mp3":         "audio/mpeg",
	".m4a":         "audio/mp4",
	".wav":         "audio/wav",
	".ogg":         "audio/ogg",
	".oga":         "audio/ogg",
	".flac":        "audio/flac",
	".pdf":         "application/pdf",
}

// PreviewURL 返回在应用内渲染 HTML 文件时使用的地址。
//
// 同一个地址也是页面里相对引用的基准，因此文件位于工作区内时以工作区作为
// 根目录，页面就能引用工作区中的共享样式或图片；单独打开的文件则以文件所在
// 目录作为根目录。
func (s *AppService) PreviewURL(workspacePath, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("不能预览目录")
	}
	root := previewRoot(workspacePath, abs)
	s.allow(root)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(root))
	escaped := (&url.URL{Path: filepath.ToSlash(rel)}).EscapedPath()
	return previewPrefix + encoded + "/" + escaped, nil
}

func previewRoot(workspacePath, abs string) string {
	dir := filepath.Dir(abs)
	workspace := strings.TrimSpace(workspacePath)
	if workspace == "" {
		return dir
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return dir
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return dir
	}
	return root
}

// previewHandler 提供 /__preview/ 下的预览资源，其余请求交给下一个处理器。
func (s *AppService) previewHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, previewPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		root, rel, err := parsePreviewPath(r.URL.Path)
		if err != nil {
			http.Error(w, "bad preview path", http.StatusBadRequest)
			return
		}
		if !s.isAllowed(root) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		if !withinDir(root, full) || !s.isAllowed(full) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		contentType, ok := previewAssetTypes[strings.ToLower(filepath.Ext(full))]
		if !ok {
			http.Error(w, "unsupported", http.StatusUnsupportedMediaType)
			return
		}
		f, err := os.Open(full)
		if err != nil {
			http.Error(w, "unreadable", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", contentType)
		// 预览是只读视图，而磁盘上的文件可能被别的程序改动，所以让浏览器每次
		// 都做一次条件请求（ServeContent 会处理 Last-Modified），而不是直接用缓存。
		w.Header().Set("Cache-Control", "private, no-cache")
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	})
}

// parsePreviewPath 从请求路径里取出预览根目录和相对路径。
func parsePreviewPath(requestPath string) (string, string, error) {
	rest := strings.TrimPrefix(requestPath, previewPrefix)
	separator := strings.IndexByte(rest, '/')
	if separator <= 0 || separator == len(rest)-1 {
		return "", "", errors.New("预览路径不完整")
	}
	raw, err := base64.RawURLEncoding.DecodeString(rest[:separator])
	if err != nil {
		return "", "", err
	}
	root := filepath.Clean(string(raw))
	if !filepath.IsAbs(root) {
		return "", "", errors.New("预览根目录无效")
	}
	return root, rest[separator+1:], nil
}

// withinDir 判断 full 是否仍然位于 root 之内，用来拦住 ../ 逃逸。
func withinDir(root, full string) bool {
	rel, err := filepath.Rel(root, full)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
