package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 更新检查相关的常量。手动检查与自动检查共用同一套逻辑，自动检查在启动后
// 延迟一小段时间执行一次，之后每 6 小时一次。
const (
	updateInitialDelay  = 10 * time.Second
	updateCheckInterval = 6 * time.Hour

	// releasesAPIURL 指向仓库的 latest release。使用 API 而不是抓取 HTML 页面，
	// 这样能同时拿到版本号与安装包列表。
	releasesAPIURL  = "https://api.github.com/repos/JiangL1011/Veda/releases/latest"
	releasesPageURL = "https://github.com/JiangL1011/Veda/releases"

	updateHTTPTimeout      = 10 * time.Minute
	maxUpdateDownloadBytes = 1 << 30
	updateUserAgent        = "Veda-Updater"
	updateProgressInterval = 120 * time.Millisecond
)

// 更新状态机。
const (
	updateStatusIdle        = "idle"
	updateStatusChecking    = "checking"
	updateStatusUpToDate    = "up-to-date"
	updateStatusDownloading = "downloading"
	updateStatusReady       = "ready"
	updateStatusUnsupported = "unsupported"
	updateStatusError       = "error"
)

// AppInfo 是“关于”页面需要的静态信息。
type AppInfo struct {
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	Arch        string `json:"arch"`
	ReleasesURL string `json:"releasesUrl"`
}

// UpdateState 是更新检查与下载的完整状态快照，供前端轮询。
type UpdateState struct {
	Status         string  `json:"status"`
	CurrentVersion string  `json:"currentVersion"`
	LatestVersion  string  `json:"latestVersion"`
	ReleaseURL     string  `json:"releaseUrl"`
	ReleaseNotes   string  `json:"releaseNotes"`
	PublishedAt    string  `json:"publishedAt"`
	AssetName      string  `json:"assetName"`
	AssetSize      int64   `json:"assetSize"`
	Downloaded     int64   `json:"downloaded"`
	Progress       float64 `json:"progress"`
	Error          string  `json:"error"`
	CheckedAt      string  `json:"checkedAt"`
}

type releaseAsset struct {
	Name string
	URL  string
	Size int64
}

type releaseInfo struct {
	Version     string
	HTMLURL     string
	Notes       string
	PublishedAt string
	Assets      []releaseAsset
}

type updater struct {
	mu        sync.Mutex
	state     UpdateState
	running   bool
	assetPath string

	client   *http.Client
	checkURL string
	goos     string
	goarch   string
	dataDir  func() (string, error)
	// version 返回当前版本，测试时可以替换。
	version func() string
}

func newUpdater() *updater {
	return &updater{
		client:   &http.Client{Timeout: updateHTTPTimeout},
		checkURL: releasesAPIURL,
		goos:     runtime.GOOS,
		goarch:   runtime.GOARCH,
		dataDir:  dataDir,
		version:  currentVersion,
		state: UpdateState{
			Status:         updateStatusIdle,
			CurrentVersion: currentVersion(),
		},
	}
}

// State 返回当前状态快照。
func (u *updater) State() UpdateState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

// Check 异步检查更新；已有任务在跑时直接返回当前状态。
func (u *updater) Check() UpdateState {
	u.mu.Lock()
	if u.running {
		state := u.state
		u.mu.Unlock()
		return state
	}
	u.running = true
	u.state.Status = updateStatusChecking
	u.state.Error = ""
	u.state.Progress = 0
	u.state.Downloaded = 0
	u.state.AssetSize = 0
	u.state.AssetName = ""
	u.state.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	state := u.state
	u.mu.Unlock()

	go u.run()
	return state
}

func (u *updater) run() {
	defer func() {
		u.mu.Lock()
		u.running = false
		u.mu.Unlock()
	}()

	current := u.version()
	u.mu.Lock()
	u.state.CurrentVersion = current
	u.mu.Unlock()

	release, err := u.fetchLatest()
	if err != nil {
		u.fail(err)
		return
	}

	u.mu.Lock()
	u.state.LatestVersion = release.Version
	u.state.ReleaseURL = release.HTMLURL
	u.state.ReleaseNotes = release.Notes
	u.state.PublishedAt = release.PublishedAt
	u.mu.Unlock()

	if release.Version == "" || compareVersions(release.Version, current) <= 0 {
		u.mu.Lock()
		u.state.Status = updateStatusUpToDate
		u.state.Progress = 0
		u.assetPath = ""
		u.mu.Unlock()
		return
	}

	asset, ok := pickUpdateAsset(release.Assets, u.goos, u.goarch)
	if !ok {
		u.mu.Lock()
		u.state.Status = updateStatusUnsupported
		u.mu.Unlock()
		return
	}

	u.mu.Lock()
	u.state.Status = updateStatusDownloading
	u.state.AssetName = asset.Name
	u.state.AssetSize = asset.Size
	u.mu.Unlock()

	path, err := u.download(asset)
	if err != nil {
		u.fail(fmt.Errorf("下载安装包失败：%w", err))
		return
	}

	u.mu.Lock()
	u.assetPath = path
	u.state.Status = updateStatusReady
	u.state.Downloaded = asset.Size
	u.state.Progress = 1
	u.mu.Unlock()
}

func (u *updater) fail(err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state.Status = updateStatusError
	u.state.Error = err.Error()
	u.state.Progress = 0
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

func (u *updater) fetchLatest() (releaseInfo, error) {
	client := u.client
	req, err := http.NewRequest(http.MethodGet, u.checkURL, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", updateUserAgent+"/"+u.version())

	resp, err := client.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return releaseInfo{}, errors.New("仓库还没有发布任何版本")
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			return releaseInfo{}, errors.New("请求过于频繁，请稍后再试")
		}
		return releaseInfo{}, fmt.Errorf("发布接口返回 %s", resp.Status)
	}

	var payload githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return releaseInfo{}, err
	}
	if payload.Draft {
		return releaseInfo{}, errors.New("最新版本还是草稿状态")
	}

	info := releaseInfo{
		Version:     normalizeVersion(payload.TagName),
		HTMLURL:     payload.HTMLURL,
		Notes:       strings.TrimSpace(payload.Body),
		PublishedAt: payload.PublishedAt,
	}
	if info.HTMLURL == "" {
		info.HTMLURL = releasesPageURL
	}
	for _, asset := range payload.Assets {
		if asset.Name == "" || asset.BrowserDownloadURL == "" {
			continue
		}
		info.Assets = append(info.Assets, releaseAsset{
			Name: asset.Name,
			URL:  asset.BrowserDownloadURL,
			Size: asset.Size,
		})
	}
	return info, nil
}

// updateAssetSuffixes 按优先级列出当前平台可用的安装包名后缀。发布流程只产出
// macOS 与 Windows 的安装包，其余平台返回空列表。
func updateAssetSuffixes(goos, goarch string) []string {
	switch goos {
	case "darwin":
		arch := "arm64"
		if goarch != "arm64" {
			arch = "x64"
		}
		return []string{
			"macos-" + arch + ".dmg",
			"macos-" + arch + ".app.zip",
		}
	case "windows":
		// Windows 只发布 x64 产物；arm64 通过系统自带的模拟层运行。
		return []string{
			"windows-x64-setup.exe",
			"windows-x64.exe",
		}
	default:
		return nil
	}
}

// pickUpdateAsset 按后缀优先级挑选安装包。
func pickUpdateAsset(assets []releaseAsset, goos, goarch string) (releaseAsset, bool) {
	for _, suffix := range updateAssetSuffixes(goos, goarch) {
		for _, asset := range assets {
			if strings.HasSuffix(strings.ToLower(asset.Name), suffix) {
				return asset, true
			}
		}
	}
	return releaseAsset{}, false
}

func (u *updater) updatesDir() (string, error) {
	dir, err := u.dataDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, "updates")
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}
	return target, nil
}

// download 把安装包下载到 ~/.veda/updates。已存在且大小一致的会被复用。
func (u *updater) download(asset releaseAsset) (string, error) {
	dir, err := u.updatesDir()
	if err != nil {
		return "", err
	}
	final := filepath.Join(dir, filepath.Base(asset.Name))
	if info, err := os.Stat(final); err == nil && !info.IsDir() {
		if asset.Size <= 0 || info.Size() == asset.Size {
			u.mu.Lock()
			u.state.Downloaded = info.Size()
			u.state.Progress = 1
			u.mu.Unlock()
			return final, nil
		}
	}

	req, err := http.NewRequest(http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", updateUserAgent+"/"+u.version())
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载接口返回 %s", resp.Status)
	}

	tmp := final + ".part"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}

	writer := &progressWriter{
		target:  file,
		total:   maxInt64(asset.Size, resp.ContentLength),
		onWrite: u.reportProgress,
	}
	written, copyErr := io.Copy(writer, io.LimitReader(resp.Body, maxUpdateDownloadBytes))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", closeErr
	}
	if asset.Size > 0 && written != asset.Size {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("安装包不完整（期望 %d 字节，实际 %d 字节）", asset.Size, written)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return final, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (u *updater) reportProgress(written, total int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.state.Downloaded = written
	if total > 0 {
		u.state.Progress = float64(written) / float64(total)
		if u.state.Progress > 1 {
			u.state.Progress = 1
		}
	}
}

// progressWriter 负责按时间节流地把下载进度同步到状态里。
type progressWriter struct {
	target    io.Writer
	total     int64
	written   int64
	last      time.Time
	onWrite   func(written, total int64)
	firstSeen bool
}

func (p *progressWriter) Write(data []byte) (int, error) {
	n, err := p.target.Write(data)
	p.written += int64(n)
	now := time.Now()
	if !p.firstSeen || now.Sub(p.last) >= updateProgressInterval {
		p.firstSeen = true
		p.last = now
		if p.onWrite != nil {
			p.onWrite(p.written, p.total)
		}
	}
	return n, err
}

// Install 启动已下载的安装包并在完成后重启应用。
func (u *updater) Install() error {
	u.mu.Lock()
	if u.state.Status != updateStatusReady || u.assetPath == "" {
		u.mu.Unlock()
		return errors.New("当前没有已下载的更新")
	}
	path := u.assetPath
	u.mu.Unlock()
	return launchUpdateInstaller(path)
}
