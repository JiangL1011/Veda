package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitForUpdateStatus(t *testing.T, u *updater, want ...string) UpdateState {
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

// testReleaseServer 模拟 GitHub 的 latest release 接口与安装包下载。
// assetName 为空时表示该版本没有任何产物。
func testReleaseServer(t *testing.T, tag, assetName string, payload []byte) (*httptest.Server, *updater) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			release := map[string]any{
				"tag_name":     tag,
				"html_url":     "https://example.test/releases/" + tag,
				"body":         "notes",
				"published_at": "2026-01-01T00:00:00Z",
				"assets":       []map[string]any{},
			}
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
	u := newUpdater()
	u.client = server.Client()
	u.checkURL = server.URL + "/release"
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

func TestUpdaterReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	u := newUpdater()
	u.client = server.Client()
	u.checkURL = server.URL
	u.dataDir = func() (string, error) { return t.TempDir(), nil }
	u.version = func() string { return "0.1.0" }

	u.Check()
	state := waitForUpdateStatus(t, u, updateStatusError)
	if state.Error == "" {
		t.Fatal("expected an error message")
	}
}

func TestInstallWithoutDownloadFails(t *testing.T) {
	u := newUpdater()
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
