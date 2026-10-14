package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func previewRequestPath(root, rel string) string {
	return previewPrefix + base64.RawURLEncoding.EncodeToString([]byte(root)) + "/" + rel
}

func previewTestService(t *testing.T) *AppService {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return newAppService()
}

func previewHandlerFor(service *AppService) http.Handler {
	return service.previewHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fallthrough", http.StatusTeapot)
	}))
}

func getPreview(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestPreviewURLUsesWorkspaceRootAndServesRelativeAssets(t *testing.T) {
	service := previewTestService(t)
	workspace := t.TempDir()
	docs := filepath.Join(workspace, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(docs, "page.html")
	if err := os.WriteFile(page, []byte("<html><link rel=\"stylesheet\" href=\"style.css\"></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	style := filepath.Join(docs, "style.css")
	if err := os.WriteFile(style, []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(docs, "notes.secret")
	if err := os.WriteFile(secret, []byte("hidden"), 0o644); err != nil {
		t.Fatal(err)
	}

	url, err := service.PreviewURL(workspace, page)
	if err != nil {
		t.Fatal(err)
	}
	want := previewRequestPath(workspace, "docs/page.html")
	if url != want {
		t.Fatalf("preview url = %q, want %q", url, want)
	}

	handler := previewHandlerFor(service)
	recorder := getPreview(t, handler, url)
	if recorder.Code != http.StatusOK {
		t.Fatalf("page status = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("page content type = %q", contentType)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "style.css") {
		t.Fatalf("page body = %q", body)
	}

	// 与页面同目录的相对引用必须能通过同一个根目录加载。
	styleURL := previewRequestPath(workspace, "docs/style.css")
	recorder = getPreview(t, handler, styleURL)
	if recorder.Code != http.StatusOK {
		t.Fatalf("style status = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/css") {
		t.Fatalf("style content type = %q", contentType)
	}

	// 白名单之外的扩展名不提供。
	recorder = getPreview(t, handler, previewRequestPath(workspace, "docs/notes.secret"))
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("secret status = %d, want 415", recorder.Code)
	}

	// 非预览前缀的请求要交回默认处理器。
	recorder = getPreview(t, handler, "/index.html")
	if recorder.Code != http.StatusTeapot {
		t.Fatalf("fallthrough status = %d, want 418", recorder.Code)
	}
}

func TestPreviewHandlerRejectsDisallowedRootAndEscapes(t *testing.T) {
	service := previewTestService(t)
	workspace := t.TempDir()
	page := filepath.Join(workspace, "page.html")
	if err := os.WriteFile(page, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := previewHandlerFor(service)
	// 还没有调用过 PreviewURL，根目录不在允许列表里。
	disallowed := t.TempDir()
	if err := os.WriteFile(filepath.Join(disallowed, "page.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := getPreview(t, handler, previewRequestPath(disallowed, "page.html"))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("disallowed status = %d, want 403", recorder.Code)
	}

	url, err := service.PreviewURL(workspace, page)
	if err != nil {
		t.Fatal(err)
	}
	if recorder = getPreview(t, handler, url); recorder.Code != http.StatusOK {
		t.Fatalf("allowed status = %d, want 200", recorder.Code)
	}

	// ../ 不能逃出预览根目录。
	escape := previewRequestPath(workspace, "../outside.txt")
	if recorder = getPreview(t, handler, escape); recorder.Code != http.StatusForbidden {
		t.Fatalf("escape status = %d, want 403", recorder.Code)
	}

	recorder = getPreview(t, handler, previewPrefix+"not-base64!/page.html")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("bad id status = %d, want 400", recorder.Code)
	}
}

func TestPreviewURLFallsBackToFileDirectoryOutsideWorkspace(t *testing.T) {
	service := previewTestService(t)
	workspace := t.TempDir()
	other := t.TempDir()
	page := filepath.Join(other, "leaf.html")
	if err := os.WriteFile(page, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	url, err := service.PreviewURL(workspace, page)
	if err != nil {
		t.Fatal(err)
	}
	if want := previewRequestPath(other, "leaf.html"); url != want {
		t.Fatalf("preview url = %q, want %q", url, want)
	}

	// 单独打开文件时没有工作区，根目录就是文件所在目录。
	url, err = service.PreviewURL("", page)
	if err != nil {
		t.Fatal(err)
	}
	if want := previewRequestPath(other, "leaf.html"); url != want {
		t.Fatalf("single file preview url = %q, want %q", url, want)
	}
}

func TestPreviewURLRejectsDirectories(t *testing.T) {
	service := previewTestService(t)
	if _, err := service.PreviewURL("", t.TempDir()); err == nil {
		t.Fatal("expected an error when previewing a directory")
	}
}

func TestPreviewHandlerInjectsShortcutBridgeIntoHTML(t *testing.T) {
	service := previewTestService(t)
	workspace := t.TempDir()
	page := filepath.Join(workspace, "page.html")
	body := "<html><body><p>hello</p></BODY></html>"
	if err := os.WriteFile(page, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	style := filepath.Join(workspace, "style.css")
	if err := os.WriteFile(style, []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := service.PreviewURL(workspace, page); err != nil {
		t.Fatal(err)
	}
	handler := previewHandlerFor(service)

	recorder := getPreview(t, handler, previewRequestPath(workspace, "page.html"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("page status = %d, want 200", recorder.Code)
	}
	served := recorder.Body.String()
	if !strings.Contains(served, "vedaPreviewKey") {
		t.Fatalf("page body is missing the shortcut bridge: %q", served)
	}
	// 脚本要插在 </body> 之前，原文不能丢。
	if at, end := strings.Index(served, previewBridge), strings.Index(served, "</BODY>"); at < 0 || end < 0 || at > end {
		t.Fatalf("bridge position = %d, body end = %d, body = %q", at, end, served)
	}
	if !strings.HasPrefix(served, "<html><body><p>hello</p>") || !strings.HasSuffix(served, "</BODY></html>") {
		t.Fatalf("page content was not preserved: %q", served)
	}

	// 非文档资源保持原样。
	recorder = getPreview(t, handler, previewRequestPath(workspace, "style.css"))
	if served := recorder.Body.String(); served != "body{}" {
		t.Fatalf("style body = %q, want it untouched", served)
	}
}

func TestInjectPreviewBridge(t *testing.T) {
	cases := []struct {
		name string
		page string
	}{
		{name: "body tag", page: "<html><body>x</body></html>"},
		{name: "uppercase body tag", page: "<html><BODY>x</BODY></html>"},
		{name: "no body tag", page: "<p>x</p>"},
		{name: "empty", page: ""},
		// 只有最后一个 </body> 是真正的结束标签，脚本不能插到字符串字面量里。
		{name: "body tag inside script", page: `<script>var tag = "</body>";</script></body>`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := string(injectPreviewBridge([]byte(test.page)))
			if strings.Count(got, previewBridge) != 1 {
				t.Fatalf("bridge count = %d in %q", strings.Count(got, previewBridge), got)
			}
			at := strings.Index(got, previewBridge)
			want := test.page
			if want == "" {
				if got != previewBridge {
					t.Fatalf("injected = %q, want only the bridge", got)
				}
				return
			}
			if last := lastIndexFoldASCII([]byte(want), "</body>"); last >= 0 {
				if at != last {
					t.Fatalf("bridge position = %d, want %d in %q", at, last, got)
				}
			} else if !strings.HasPrefix(got, want) {
				t.Fatalf("injected = %q, want it to keep %q as a prefix", got, want)
			}
			if strings.Join(strings.Split(got, previewBridge), "") != want {
				t.Fatalf("injected = %q, want the page unchanged around the bridge", got)
			}
		})
	}
}
