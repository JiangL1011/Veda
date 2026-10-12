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
