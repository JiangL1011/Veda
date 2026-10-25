package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
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

// previewBridge 是注入到被预览 HTML 里的一段桥接脚本。
//
// 预览页面在 iframe 里运行，是独立的浏览上下文：焦点落在预览里时，按键不会冒泡到
// 应用页面，窗口级的快捷键（关闭标签页、查找……）会全部失效。所以这里把按键转发给
// 应用页面，由它按同一套快捷键设置处理，预览与 Markdown 的行为保持一致。
//
// 脚本用 CDATA 注释包住，这样注入 HTML 与 XHTML 两种文档都是合法的；开头的幂等标记
// 保证同一个文档里注入多次也只注册一次监听。消息的接收端在
// frontend/src/lib/shortcuts.ts 的 previewShortcutEvent。
const previewBridge = `<script>//<![CDATA[
(function () {
  if (window.__vedaPreviewBridge) {
    return;
  }
  window.__vedaPreviewBridge = true;
  /* 只转发带修饰键的组合：应用里的快捷键都需要修饰键，纯字符按键留给预览页面自己。 */
  window.addEventListener("keydown", function (event) {
    if (!event.metaKey && !event.ctrlKey && !event.altKey) {
      return;
    }
    try {
      top.postMessage({
        vedaPreviewKey: {
          key: event.key,
          code: event.code,
          metaKey: event.metaKey,
          ctrlKey: event.ctrlKey,
          altKey: event.altKey,
          shiftKey: event.shiftKey
        }
      }, "*");
    } catch (err) {
      /* 转发失败不能影响预览页面本身。 */
    }
  }, true);
})();
//]]></script>`

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

// PreviewHandler 提供 /__preview/ 下的预览资源，其余请求交给下一个处理器。
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
		if isPreviewHTML(contentType) {
			page, err := io.ReadAll(f)
			if err != nil {
				http.Error(w, "unreadable", http.StatusInternalServerError)
				return
			}
			http.ServeContent(w, r, info.Name(), info.ModTime(), bytes.NewReader(injectPreviewBridge(page)))
			return
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), f)
	})
}

// isPreviewHTML 判断响应会不会被浏览器当作文档解析，只有这类文件需要注入桥接脚本。
func isPreviewHTML(contentType string) bool {
	return strings.HasPrefix(contentType, "text/html") || strings.HasPrefix(contentType, "application/xhtml+xml")
}

// injectPreviewBridge 把桥接脚本插到最后一个 </body> 之前；没有 </body> 时追加到
// 文档末尾。插入按字节进行、不重新编码，所以多字节编码的页面也不会被破坏。
func injectPreviewBridge(page []byte) []byte {
	out := make([]byte, 0, len(page)+len(previewBridge))
	if at := lastIndexFoldASCII(page, "</body>"); at >= 0 {
		out = append(out, page[:at]...)
		out = append(out, previewBridge...)
		return append(out, page[at:]...)
	}
	out = append(out, page...)
	return append(out, previewBridge...)
}

// lastIndexFoldASCII 返回 needle 在 data 里最后一次出现的位置，大小写不敏感。
func lastIndexFoldASCII(data []byte, needle string) int {
	for i := len(data) - len(needle); i >= 0; i-- {
		if hasFoldASCII(data[i:], needle) {
			return i
		}
	}
	return -1
}

func hasFoldASCII(data []byte, needle string) bool {
	for i := 0; i < len(needle); i++ {
		if lowerASCII(data[i]) != needle[i] {
			return false
		}
	}
	return true
}

func lowerASCII(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
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

// PreviewHandler 返回挂载 /__preview/ 预览路由的中间件。
func PreviewHandler(s *AppService) func(http.Handler) http.Handler {
	return s.previewHandler
}
