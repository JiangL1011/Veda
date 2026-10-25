package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"veda/internal/model"
)

func waitForUpdateStatus(t *testing.T, u *Updater, want ...string) model.UpdateState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := u.State()
		for _, candidate := range want {
			if state.Status == candidate {
				return state
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	state := u.State()
	t.Fatalf("update status = %q, want one of %v (error: %s)", state.Status, want, state.Error)
	return state
}

// checkAndWait 等 Updater 空闲后触发一次检查，并等它跑完。上一次检查刚结束时
// running 可能还没复位，直接调 Check 会拿到旧状态，所以这里重试到真正启动为止。
func checkAndWait(t *testing.T, u *Updater, want ...string) model.UpdateState {
	t.Helper()
	return checkWith(t, u, u.Check, want...)
}

// manualCheckAndWait 同 checkAndWait，但走手动检查（会忽略缓存里的限流窗口）。
func manualCheckAndWait(t *testing.T, u *Updater, want ...string) model.UpdateState {
	t.Helper()
	return checkWith(t, u, u.CheckManually, want...)
}

func checkWith(t *testing.T, u *Updater, check func() model.UpdateState, want ...string) model.UpdateState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if state := check(); state.Status == updateStatusChecking {
			return waitForUpdateStatus(t, u, want...)
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("Updater never became idle")
	return model.UpdateState{}
}

// newUpdaterForTest 构造一个不访问真实网络的空更新器，测试会覆盖各个字段。
func newUpdaterForTest() *Updater {
	return New(func() string { return "0.1.0" }, func() (string, error) { return "", nil })
}

// newTestUpdater 构造指向测试服务器、并把检查缓存写进 dir 的 Updater。
func newTestUpdater(server *httptest.Server, dir string) *Updater {
	u := newUpdaterForTest()
	u.client = server.Client()
	u.checkURL = server.URL
	u.pageURL = server.URL + "/releases"
	u.dataDir = func() (string, error) { return dir, nil }
	u.version = func() string { return "0.1.0" }
	return u
}

// releasePayload 构造 latest release 接口的响应体。没有产物时 assets 为空数组。
func releasePayload(tag string, assets ...map[string]any) map[string]any {
	if assets == nil {
		assets = []map[string]any{}
	}
	return map[string]any{
		"tag_name":     tag,
		"html_url":     "https://example.test/releases/" + tag,
		"body":         "notes",
		"published_at": "2026-01-01T00:00:00Z",
		"assets":       assets,
	}
}

// testReleaseServer 模拟 GitHub 的 latest release 接口与安装包下载。
// assetName 为空时表示该版本没有任何产物。
func testReleaseServer(t *testing.T, tag, assetName string, payload []byte) (*httptest.Server, *Updater) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			release := releasePayload(tag)
			if assetName != "" {
				release["assets"] = []map[string]any{{
					"name":                 assetName,
					"browser_download_url": "http://" + r.Host + "/asset",
					"size":                 len(payload),
				}}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(release)
		case "/asset":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))

	dir := t.TempDir()
	u := newUpdaterForTest()
	u.client = server.Client()
	u.checkURL = server.URL + "/release"
	u.pageURL = server.URL + "/releases"
	u.goos = "darwin"
	u.goarch = "arm64"
	u.dataDir = func() (string, error) { return dir, nil }
	u.version = func() string { return "0.1.0" }
	return server, u
}

func TestUpdateAssetSuffixes(t *testing.T) {
	cases := []struct {
		goos    string
		goarch  string
		want    string
		hasSome bool
	}{
		{"darwin", "arm64", "macos-arm64.dmg", true},
		{"darwin", "amd64", "macos-x64.dmg", true},
		{"windows", "amd64", "windows-x64-setup.exe", true},
		{"windows", "arm64", "windows-x64-setup.exe", true},
		{"linux", "amd64", "", false},
	}
	for _, tc := range cases {
		suffixes := updateAssetSuffixes(tc.goos, tc.goarch)
		if tc.hasSome {
			if len(suffixes) == 0 || suffixes[0] != tc.want {
				t.Errorf("updateAssetSuffixes(%q, %q) = %v, want first %q", tc.goos, tc.goarch, suffixes, tc.want)
			}
			continue
		}
		if len(suffixes) != 0 {
			t.Errorf("updateAssetSuffixes(%q, %q) = %v, want empty", tc.goos, tc.goarch, suffixes)
		}
	}
}

func TestPickUpdateAssetPrefersPlatformInstaller(t *testing.T) {
	assets := []releaseAsset{
		{Name: "Veda-0.2.0-macos-x64.dmg"},
		{Name: "Veda-0.2.0-macos-arm64.app.zip"},
		{Name: "Veda-0.2.0-macos-arm64.dmg"},
		{Name: "Veda-0.2.0-windows-x64.exe"},
		{Name: "Veda-0.2.0-windows-x64-setup.exe"},
	}

	got, ok := pickUpdateAsset(assets, "darwin", "arm64")
	if !ok || got.Name != "Veda-0.2.0-macos-arm64.dmg" {
		t.Fatalf("darwin/arm64 picked %q (%v)", got.Name, ok)
	}
	got, ok = pickUpdateAsset(assets, "darwin", "amd64")
	if !ok || got.Name != "Veda-0.2.0-macos-x64.dmg" {
		t.Fatalf("darwin/amd64 picked %q (%v)", got.Name, ok)
	}
	got, ok = pickUpdateAsset(assets, "windows", "amd64")
	if !ok || got.Name != "Veda-0.2.0-windows-x64-setup.exe" {
		t.Fatalf("windows/amd64 picked %q (%v)", got.Name, ok)
	}
	if _, ok := pickUpdateAsset(assets, "linux", "amd64"); ok {
		t.Fatal("linux should have no automatic installer")
	}
}

func TestPickUpdateAssetFallsBackToAppZip(t *testing.T) {
	assets := []releaseAsset{{Name: "Veda-0.2.0-macos-arm64.app.zip"}}
	got, ok := pickUpdateAsset(assets, "darwin", "arm64")
	if !ok || got.Name != "Veda-0.2.0-macos-arm64.app.zip" {
		t.Fatalf("picked %q (%v)", got.Name, ok)
	}
}

func TestUpdaterDownloadsNewerRelease(t *testing.T) {
	payload := []byte("installer-bytes")
	server, u := testReleaseServer(t, "v0.2.0", "Veda-0.2.0-macos-arm64.dmg", payload)
	defer server.Close()

	if state := u.Check(); state.Status != updateStatusChecking {
		t.Fatalf("Check() status = %q, want %q", state.Status, updateStatusChecking)
	}
	final := waitForUpdateStatus(t, u, updateStatusReady, updateStatusError)
	if final.Status != updateStatusReady {
		t.Fatalf("status = %q, error = %q", final.Status, final.Error)
	}
	if final.LatestVersion != "0.2.0" {
		t.Errorf("LatestVersion = %q, want %q", final.LatestVersion, "0.2.0")
	}
	if final.CurrentVersion != "0.1.0" {
		t.Errorf("CurrentVersion = %q, want %q", final.CurrentVersion, "0.1.0")
	}
	if final.Progress != 1 || final.Downloaded != int64(len(payload)) {
		t.Errorf("progress = %v, downloaded = %d, want 1 / %d", final.Progress, final.Downloaded, len(payload))
	}

	dir, _ := u.dataDir()
	raw, err := os.ReadFile(filepath.Join(dir, "updates", "Veda-0.2.0-macos-arm64.dmg"))
	if err != nil {
		t.Fatalf("downloaded file missing: %v", err)
	}
	if string(raw) != string(payload) {
		t.Fatalf("downloaded content = %q, want %q", raw, payload)
	}
}

func TestUpdaterReusesDownloadedInstaller(t *testing.T) {
	payload := []byte("installer-bytes")
	server, u := testReleaseServer(t, "v0.2.0", "Veda-0.2.0-macos-arm64.dmg", payload)
	defer server.Close()

	dir, _ := u.dataDir()
	target := filepath.Join(dir, "updates")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(target, "Veda-0.2.0-macos-arm64.dmg")
	if err := os.WriteFile(existing, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	u.Check()
	final := waitForUpdateStatus(t, u, updateStatusReady, updateStatusError)
	if final.Status != updateStatusReady {
		t.Fatalf("status = %q, error = %q", final.Status, final.Error)
	}
}

func TestUpdaterReportsUpToDate(t *testing.T) {
	server, u := testReleaseServer(t, "v0.1.0", "", nil)
	defer server.Close()

	u.Check()
	state := waitForUpdateStatus(t, u, updateStatusUpToDate, updateStatusError)
	if state.Status != updateStatusUpToDate {
		t.Fatalf("status = %q, error = %q", state.Status, state.Error)
	}
}

func TestUpdaterReportsUnsupportedPlatform(t *testing.T) {
	server, u := testReleaseServer(t, "v9.9.9", "Veda-9.9.9-windows-x64.exe", []byte("x"))
	defer server.Close()

	u.goos = "linux"
	u.Check()
	state := waitForUpdateStatus(t, u, updateStatusUnsupported, updateStatusError)
	if state.Status != updateStatusUnsupported {
		t.Fatalf("status = %q, error = %q", state.Status, state.Error)
	}
}

// TestUpdaterReportsThrottlingWhenAllPathsFail 验证发布接口与 releases 页面都失败时，
// 只给用户一句统一的提示——前端会渲染成「检查更新失败：请求过于频繁，请稍后再试」。
func TestUpdaterReportsThrottlingWhenAllPathsFail(t *testing.T) {
	cases := []struct {
		name string
		api  http.HandlerFunc
	}{
		{"发布接口限流", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10))
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4."}`))
		}},
		{"发布接口报错", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/releases") {
					http.Error(w, "boom", http.StatusInternalServerError)
					return
				}
				tc.api(w, r)
			}))
			defer server.Close()

			u := newTestUpdater(server, t.TempDir())
			state := checkAndWait(t, u, updateStatusError)
			if state.Error != updateRateLimitError {
				t.Fatalf("error = %q, want %q", state.Error, updateRateLimitError)
			}
		})
	}
}

// TestFetchLatestDoesNotLockOnPlainForbidden 验证代理或防火墙返回的 403 不会被误判成
// 限流：错误里带 HTTP 状态，也不会记下限流窗口把后续检查锁死。
func TestFetchLatestDoesNotLockOnPlainForbidden(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Forbidden"}`))
	}))
	defer server.Close()

	u := newTestUpdater(server, t.TempDir())
	for call := 1; call <= 2; call++ {
		_, err := u.fetchLatest(false)
		if err == nil {
			t.Fatalf("call %d: expected an error", call)
		}
		if !strings.Contains(err.Error(), "403") {
			t.Fatalf("call %d: error = %q, want the HTTP status", call, err)
		}
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2 (a plain 403 must not lock later checks)", got)
	}
}

func TestInstallWithoutDownloadFails(t *testing.T) {
	u := newUpdaterForTest()
	if err := u.Install(); err == nil {
		t.Fatal("Install() without a downloaded update should fail")
	}
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	if got := shellQuote("/tmp/a'b"); got != `'/tmp/a'\''b'` {
		t.Fatalf("shellQuote = %q", got)
	}
}

func TestMacOSBundlePath(t *testing.T) {
	got, err := macOSBundlePath("/Applications/Veda.app/Contents/MacOS/Veda")
	if err != nil || got != "/Applications/Veda.app" {
		t.Fatalf("macOSBundlePath = %q, %v", got, err)
	}
	if _, err := macOSBundlePath("/usr/local/bin/veda"); err == nil {
		t.Fatal("expected an error outside an .app bundle")
	}
}

// TestUpdaterUsesConditionalRequest 验证第二次检查带上 ETag，并在 304 时复用本地缓存：
// 缓存跨进程存活，重启应用也能命中 304，省下响应体的传输。
func TestUpdaterUsesConditionalRequest(t *testing.T) {
	const etag = `W/"release-1"`
	var requests, conditional atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("If-None-Match") == etag {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", etag)
		w.Header().Set("X-RateLimit-Remaining", "59")
		_ = json.NewEncoder(w).Encode(releasePayload("v0.1.0"))
	}))
	defer server.Close()

	dir := t.TempDir()
	for run := 1; run <= 2; run++ {
		u := newTestUpdater(server, dir)
		state := checkAndWait(t, u, updateStatusUpToDate, updateStatusError)
		if state.Status != updateStatusUpToDate {
			t.Fatalf("run %d: status = %q, error = %q", run, state.Status, state.Error)
		}
		if state.LatestVersion != "0.1.0" {
			t.Fatalf("run %d: LatestVersion = %q, want %q", run, state.LatestVersion, "0.1.0")
		}
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
	if got := conditional.Load(); got != 1 {
		t.Fatalf("conditional requests = %d, want 1", got)
	}
}

// TestUpdaterSkipsRequestsWhileRateLimited 验证已知的限流窗口内不再请求发布接口：
// 没有缓存可用时改抓 releases 页面，两条路都失败就给出统一提示。
func TestUpdaterSkipsRequestsWhileRateLimited(t *testing.T) {
	var apiRequests, pageRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10))
		if strings.HasPrefix(r.URL.Path, "/releases") {
			pageRequests.Add(1)
		} else {
			apiRequests.Add(1)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4."}`))
	}))
	defer server.Close()

	u := newTestUpdater(server, t.TempDir())
	state := checkAndWait(t, u, updateStatusError)
	if state.Error != updateRateLimitError {
		t.Fatalf("error = %q, want %q", state.Error, updateRateLimitError)
	}

	state = checkAndWait(t, u, updateStatusError)
	if state.Error != updateRateLimitError {
		t.Fatalf("second error = %q, want %q", state.Error, updateRateLimitError)
	}
	if got := apiRequests.Load(); got != 1 {
		t.Fatalf("api requests = %d, want 1 (the second check must stay inside the known window)", got)
	}
	if got := pageRequests.Load(); got != 2 {
		t.Fatalf("page requests = %d, want 2 (the page is the fallback for both checks)", got)
	}
}

// TestUpdaterFallsBackToCacheWhenRateLimited 验证被限流时复用上一次的检查结果。
func TestUpdaterFallsBackToCacheWhenRateLimited(t *testing.T) {
	var requests atomic.Int64
	var limited atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if limited.Load() {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10))
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4."}`))
			return
		}
		w.Header().Set("ETag", `W/"release-1"`)
		_ = json.NewEncoder(w).Encode(releasePayload("v0.2.0"))
	}))
	defer server.Close()

	u := newTestUpdater(server, t.TempDir())
	// v0.2.0 没有当前平台的安装包，因此第一次检查以「暂不支持自动安装」结束。
	state := checkAndWait(t, u, updateStatusUnsupported, updateStatusError)
	if state.Status != updateStatusUnsupported || state.LatestVersion != "0.2.0" {
		t.Fatalf("status = %q, version = %q, error = %q", state.Status, state.LatestVersion, state.Error)
	}

	// 服务端开始限流：请求虽然失败，但缓存里的结果仍然可用。
	limited.Store(true)
	state = checkAndWait(t, u, updateStatusUnsupported, updateStatusError)
	if state.Status != updateStatusUnsupported || state.LatestVersion != "0.2.0" {
		t.Fatalf("status = %q, version = %q, error = %q, want the cached result", state.Status, state.LatestVersion, state.Error)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}

	// 限流窗口已经记下：再检查一次不会发请求。
	state = checkAndWait(t, u, updateStatusUnsupported, updateStatusError)
	if state.Status != updateStatusUnsupported {
		t.Fatalf("status = %q, error = %q, want the cached result", state.Status, state.Error)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests after the limited window = %d, want 2", got)
	}
}

// TestUpdaterFallsBackToReleasesPage 验证发布接口被限流时改抓 releases 页面，
// 并用页面上解析出来的直链完成下载。
func TestUpdaterFallsBackToReleasesPage(t *testing.T) {
	payload := []byte("installer-bytes")
	const assetPath = "/JiangL1011/Veda/releases/download/v0.2.0/Veda-0.2.0-macos-arm64.dmg"

	var apiRequests, pageRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			apiRequests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4."}`))
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v0.2.0", http.StatusFound)
		case "/releases/tag/v0.2.0":
			pageRequests.Add(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Veda 0.2.0</body></html>"))
		case "/releases/expanded_assets/v0.2.0":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<li><a href="` + assetPath + `">Veda-0.2.0-macos-arm64.dmg</a></li>`))
		case assetPath:
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	u := newTestUpdater(server, dir)
	u.checkURL = server.URL + "/release"
	u.goos = "darwin"
	u.goarch = "arm64"

	state := checkAndWait(t, u, updateStatusReady, updateStatusError)
	if state.Status != updateStatusReady {
		t.Fatalf("status = %q, error = %q", state.Status, state.Error)
	}
	if state.LatestVersion != "0.2.0" {
		t.Fatalf("LatestVersion = %q, want %q", state.LatestVersion, "0.2.0")
	}
	if got := apiRequests.Load(); got != 1 {
		t.Fatalf("api requests = %d, want 1", got)
	}
	if got := pageRequests.Load(); got != 1 {
		t.Fatalf("page requests = %d, want 1", got)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "updates", "Veda-0.2.0-macos-arm64.dmg"))
	if err != nil {
		t.Fatalf("downloaded file missing: %v", err)
	}
	if string(raw) != string(payload) {
		t.Fatalf("downloaded content = %q, want %q", raw, payload)
	}
}

// TestUpdaterManualCheckIgnoresRateLimitCache 验证限流窗口内：自动检查复用缓存、
// 不碰页面，而手动检查忽略缓存，改抓 releases 页面拿到真实结果。
func TestUpdaterManualCheckIgnoresRateLimitCache(t *testing.T) {
	var limited atomic.Bool
	var pageRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			w.Header().Set("Content-Type", "application/json")
			if limited.Load() {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10))
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4."}`))
				return
			}
			// 配额刚好用尽：结果进缓存，同时记下限流窗口。
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10))
			_ = json.NewEncoder(w).Encode(releasePayload("v0.1.0"))
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v0.2.0", http.StatusFound)
		case "/releases/tag/v0.2.0":
			pageRequests.Add(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Veda 0.2.0</body></html>"))
		case "/releases/expanded_assets/v0.2.0":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<div>没有产物</div>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	u := newTestUpdater(server, t.TempDir())
	u.checkURL = server.URL + "/release"

	state := checkAndWait(t, u, updateStatusUpToDate, updateStatusError)
	if state.Status != updateStatusUpToDate {
		t.Fatalf("first check: status = %q, error = %q", state.Status, state.Error)
	}

	limited.Store(true)
	state = checkAndWait(t, u, updateStatusUpToDate, updateStatusError)
	if state.Status != updateStatusUpToDate || state.LatestVersion != "0.1.0" {
		t.Fatalf("automatic check: status = %q, version = %q, error = %q, want the cached result",
			state.Status, state.LatestVersion, state.Error)
	}
	if got := pageRequests.Load(); got != 0 {
		t.Fatalf("page requests = %d, want 0 (the automatic check should reuse the cache)", got)
	}

	// 手动检查不认缓存里的限流窗口，改抓页面拿到了 v0.2.0。
	state = manualCheckAndWait(t, u, updateStatusUnsupported, updateStatusError)
	if state.LatestVersion != "0.2.0" {
		t.Fatalf("manual check: version = %q, status = %q, error = %q",
			state.LatestVersion, state.Status, state.Error)
	}
	if got := pageRequests.Load(); got != 1 {
		t.Fatalf("page requests = %d, want 1", got)
	}
}

// TestUpdaterManualCheckScrapesPageAfterThrottling 验证「限流窗口未知但已经有缓存」时，
// 手动检查也不会拿旧缓存交差：发布接口 403 后照样改抓 releases 页面。
func TestUpdaterManualCheckScrapesPageAfterThrottling(t *testing.T) {
	var limited atomic.Bool
	var pageRequests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			w.Header().Set("Content-Type", "application/json")
			if limited.Load() {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(30*time.Minute).Unix(), 10))
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4."}`))
				return
			}
			// 这次检查成功且没有限流信息，于是缓存有内容、限流窗口仍是未知。
			_ = json.NewEncoder(w).Encode(releasePayload("v0.1.0"))
		case "/releases/latest":
			http.Redirect(w, r, "/releases/tag/v0.2.0", http.StatusFound)
		case "/releases/tag/v0.2.0":
			pageRequests.Add(1)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Veda 0.2.0</body></html>"))
		case "/releases/expanded_assets/v0.2.0":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<div>没有产物</div>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	u := newTestUpdater(server, t.TempDir())
	u.checkURL = server.URL + "/release"

	state := checkAndWait(t, u, updateStatusUpToDate, updateStatusError)
	if state.Status != updateStatusUpToDate {
		t.Fatalf("first check: status = %q, error = %q", state.Status, state.Error)
	}

	limited.Store(true)
	state = manualCheckAndWait(t, u, updateStatusUnsupported, updateStatusError)
	if state.LatestVersion != "0.2.0" {
		t.Fatalf("manual check: version = %q, status = %q, error = %q",
			state.LatestVersion, state.Status, state.Error)
	}
	if got := pageRequests.Load(); got != 1 {
		t.Fatalf("page requests = %d, want 1", got)
	}
}

func TestReleaseTagFromURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/JiangL1011/Veda/releases/tag/v0.2.0": "v0.2.0",
		"https://github.com/JiangL1011/Veda/releases/latest":     "",
		"https://github.com/JiangL1011/Veda/releases":            "",
	}
	for raw, want := range cases {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("url.Parse(%q) = %v", raw, err)
		}
		if got := releaseTagFromURL(u); got != want {
			t.Errorf("releaseTagFromURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestParseReleaseAssets(t *testing.T) {
	base, err := url.Parse("https://github.com/JiangL1011/Veda/releases/tag/v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	page := []byte(`
		<li><a href="/JiangL1011/Veda/releases/download/v0.2.0/Veda-0.2.0-macos-arm64.dmg">Veda-0.2.0-macos-arm64.dmg</a></li>
		<li><a href="/JiangL1011/Veda/releases/download/v0.2.0/Veda%20portable.app.zip">Veda portable.app.zip</a></li>
		<li><a href="/JiangL1011/Veda/releases/download/v0.1.0/Veda-0.1.0-macos-arm64.dmg">旧版本</a></li>
		<li><a href="/JiangL1011/Veda/archive/refs/tags/v0.2.0.zip">Source code</a></li>
	`)

	assets := parseReleaseAssets(page, base, "v0.2.0")
	wantNames := []string{"Veda-0.2.0-macos-arm64.dmg", "Veda portable.app.zip"}
	if len(assets) != len(wantNames) {
		t.Fatalf("assets = %+v, want %d entries", assets, len(wantNames))
	}
	for i, want := range wantNames {
		if assets[i].Name != want {
			t.Errorf("assets[%d].Name = %q, want %q", i, assets[i].Name, want)
		}
	}
	if want := "https://github.com" + "/JiangL1011/Veda/releases/download/v0.2.0/Veda-0.2.0-macos-arm64.dmg"; assets[0].URL != want {
		t.Errorf("assets[0].URL = %q, want %q", assets[0].URL, want)
	}
	if want := "https://github.com" + "/JiangL1011/Veda/releases/download/v0.2.0/Veda%20portable.app.zip"; assets[1].URL != want {
		t.Errorf("assets[1].URL = %q, want %q", assets[1].URL, want)
	}
}

func TestIsRateLimitResponse(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"主限流", `{"message":"API rate limit exceeded for 1.2.3.4."}`, true},
		{"次级限流", `{"message":"You have exceeded a secondary rate limit."}`, true},
		{"普通 403", `{"message":"Forbidden"}`, false},
		{"代理页面", `<html><body>403 Forbidden</body></html>`, false},
		{"空响应体", ``, false},
	}
	for _, tc := range cases {
		if got := isRateLimitResponse([]byte(tc.body)); got != tc.want {
			t.Errorf("isRateLimitResponse(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
