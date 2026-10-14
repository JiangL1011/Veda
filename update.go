package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
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

	// releasesLatestSuffix 与 releasesAssetsSuffix 拼在 releasesPageURL 后面，用来在
	// 发布接口不可用时改抓页面：/releases/latest 会跳到具体版本，页面上的安装包列表
	// 则由服务端渲染的 fragments 片段提供。github.com 的页面请求不占用 API 调用配额。
	releasesLatestSuffix = "/latest"
	releasesAssetsSuffix = "/expanded_assets/"

	updateHTTPTimeout      = 10 * time.Minute
	maxUpdateDownloadBytes = 1 << 30
	updateUserAgent        = "Veda-Updater"
	updateProgressInterval = 120 * time.Millisecond

	// updateCheckCacheFile 缓存最近一次发布接口的响应：ETag 用于条件请求，
	// payload 在配额用尽时可以直接复用。
	updateCheckCacheFile = "update-check.json"
	// downloadPartSuffix 是下载中的临时文件后缀，写完后会改名成正式安装包。
	downloadPartSuffix = ".part"
	// updateWorkDirPrefix 与 update_apply.go 里创建安装助手临时目录时用的前缀一致；
	// updateWorkDirStaleAge 决定多久没被动过的助手目录才算残留。
	updateWorkDirPrefix   = "veda-update-"
	updateWorkDirStaleAge = time.Hour
	// updateRateLimitCeiling 限制退避窗口的长度，避免异常的响应头把检查长时间锁死。
	updateRateLimitCeiling = time.Hour
	// updateRateLimitFallback 是被限流但没有给出重置时间时的默认退避窗口。
	updateRateLimitFallback = 5 * time.Minute
	// updateRateLimitError 是被限流时给用户看的提示。
	updateRateLimitError = "请求过于频繁，请稍后再试"
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
	// pageURL 是 releases 页面的基地址，发布接口不可用时改抓它。
	pageURL string
	goos    string
	goarch  string
	dataDir func() (string, error)
	// version 返回当前版本，测试时可以替换。
	version func() string
}

func newUpdater() *updater {
	return &updater{
		client:   &http.Client{Timeout: updateHTTPTimeout},
		checkURL: releasesAPIURL,
		pageURL:  releasesPageURL,
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

// Check 异步检查更新，自动检查走发布接口的缓存策略，已有任务在跑时直接返回当前状态。
func (u *updater) Check() UpdateState {
	return u.check(false)
}

// CheckManually 手动检查更新：忽略缓存里记下的限流窗口，尽量拿到一次真实的结果，
// 发布接口用不了时改抓 releases 页面。
func (u *updater) CheckManually() UpdateState {
	return u.check(true)
}

func (u *updater) check(manual bool) UpdateState {
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

	go u.run(manual)
	return state
}

func (u *updater) run(manual bool) {
	defer func() {
		u.mu.Lock()
		u.running = false
		u.mu.Unlock()
	}()

	current := u.version()
	u.mu.Lock()
	u.state.CurrentVersion = current
	u.mu.Unlock()

	release, err := u.checkRelease(manual)
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

// updateCheckCache 缓存最近一次发布接口的响应。ETag 用于条件请求（内容没变时
// 服务端回 304，省掉响应体传输），payload 则在配额用尽时直接复用。
// 注意：实测这个接口的 304 同样会让 X-RateLimit-Remaining 减 1，因此条件请求只省
// 流量、不省配额；真正省配额的是下面的限流窗口与 releases 页面兜底。
type updateCheckCache struct {
	ETag    string          `json:"etag,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	// LimitedUntil 是已知的限流窗口结束时间（Unix 秒）；大于 0 且未到期时不再发请求。
	LimitedUntil int64 `json:"limitedUntil,omitempty"`
}

func (c updateCheckCache) limited(now time.Time) bool {
	return c.LimitedUntil > 0 && now.Unix() < c.LimitedUntil
}

// cachedRelease 解析缓存里的响应体，没有可用缓存时返回 false。
func (c updateCheckCache) cachedRelease() (releaseInfo, bool) {
	if len(c.Payload) == 0 {
		return releaseInfo{}, false
	}
	info, err := parseRelease(c.Payload)
	if err != nil {
		return releaseInfo{}, false
	}
	return info, true
}

func (u *updater) checkCachePath() (string, error) {
	dir, err := u.dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, updateCheckCacheFile), nil
}

func (u *updater) loadCheckCache() updateCheckCache {
	path, err := u.checkCachePath()
	if err != nil {
		return updateCheckCache{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return updateCheckCache{}
	}
	var cache updateCheckCache
	if err := json.Unmarshal(raw, &cache); err != nil {
		return updateCheckCache{}
	}
	return cache
}

// saveCheckCache 落盘缓存；写失败只影响下次请求的成本，不影响本次检查结果。
func (u *updater) saveCheckCache(cache updateCheckCache) {
	path, err := u.checkCachePath()
	if err != nil {
		return
	}
	_ = writeJSONFile(path, cache)
}

// checkRelease 依次尝试发布接口与 releases 页面。发布接口数据完整，但受 GitHub 的
// 匿名调用配额限制；配额用尽、被拦截或出错时改抓页面，抓页面不占用 API 配额。
// 两条路都失败时只向上层抛出一句统一的提示。
func (u *updater) checkRelease(manual bool) (releaseInfo, error) {
	release, apiErr := u.fetchLatest(manual)
	if apiErr == nil {
		return release, nil
	}

	release, pageErr := u.fetchLatestFromPage()
	if pageErr == nil {
		return release, nil
	}

	log.Printf("veda: 检查更新失败（发布接口：%v；发布页面：%v）", apiErr, pageErr)
	return releaseInfo{}, errors.New(updateRateLimitError)
}

func (u *updater) fetchLatest(manual bool) (releaseInfo, error) {
	cache := u.loadCheckCache()
	now := time.Now()

	// 已知的限流窗口内不再发注定失败的请求：自动检查复用缓存，手动检查则跳过
	// 发布接口，交给上层的 releases 页面兜底。
	if cache.limited(now) {
		if !manual {
			if info, ok := cache.cachedRelease(); ok {
				return info, nil
			}
		}
		return releaseInfo{}, errors.New(updateRateLimitError)
	}

	req, err := http.NewRequest(http.MethodGet, u.checkURL, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", updateUserAgent+"/"+u.version())
	// 条件请求：内容没变时服务端返回 304，省掉响应体的传输。
	if cache.ETag != "" {
		req.Header.Set("If-None-Match", cache.ETag)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		info, ok := cache.cachedRelease()
		if !ok {
			return releaseInfo{}, errors.New("发布接口返回 304，但本地没有可用的缓存")
		}
		u.rememberRateLimit(&cache, resp, now, false)
		u.saveCheckCache(cache)
		return info, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return releaseInfo{}, err
	}

	if resp.StatusCode == http.StatusNotFound {
		// 仓库还没有 release：清掉旧响应，避免限流时复用已经失效的数据。
		cache.Payload = nil
		cache.ETag = ""
		u.saveCheckCache(cache)
		return releaseInfo{}, errors.New("仓库还没有发布任何版本")
	}
	if resp.StatusCode != http.StatusOK {
		// 403 也可能是代理或防火墙拦截，只有响应体说明是限流才按限流处理。
		rateLimited := (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
			isRateLimitResponse(body)
		u.rememberRateLimit(&cache, resp, now, rateLimited)
		u.saveCheckCache(cache)
		if rateLimited {
			// 缓存只在自动检查里兜底；手动检查要的是一次真实结果，交给页面。
			if !manual {
				if info, ok := cache.cachedRelease(); ok {
					return info, nil
				}
			}
			return releaseInfo{}, errors.New(updateRateLimitError)
		}
		return releaseInfo{}, fmt.Errorf("发布接口返回 %s", resp.Status)
	}

	info, err := parseRelease(body)
	if err != nil {
		return releaseInfo{}, err
	}
	cache.Payload = body
	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
		cache.ETag = etag
	}
	u.rememberRateLimit(&cache, resp, now, false)
	u.saveCheckCache(cache)
	return info, nil
}

// releaseAssetLinkRE 匹配 release 页面上安装包直链的 href。
var releaseAssetLinkRE = regexp.MustCompile(`href="([^"]*/releases/download/[^"]+)"`)

// fetchLatestFromPage 改抓 releases 页面：/releases/latest 会重定向到具体版本，
// 再从该版本的安装包列表片段里取出直链。这条路只访问 github.com，不占 API 配额。
func (u *updater) fetchLatestFromPage() (releaseInfo, error) {
	base := strings.TrimSuffix(u.pageURL, "/")
	resp, err := u.releasePageRequest(base + releasesLatestSuffix)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return releaseInfo{}, errors.New("仓库还没有发布任何版本")
	}
	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("发布页面返回 %s", resp.Status)
	}

	tag := releaseTagFromURL(resp.Request.URL)
	if tag == "" {
		return releaseInfo{}, errors.New("发布页面没有跳转到具体版本")
	}
	// 页面内容用不到，读完只是为了复用连接。
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	assets, err := u.fetchReleaseAssets(base, tag, resp.Request.URL)
	if err != nil {
		return releaseInfo{}, err
	}
	return releaseInfo{
		Version: normalizeVersion(tag),
		HTMLURL: resp.Request.URL.String(),
		Assets:  assets,
	}, nil
}

// releasePageRequest 发一个普通 GET 去抓 github.com 的页面（不带 API 相关的请求头）。
func (u *updater) releasePageRequest(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("User-Agent", updateUserAgent+"/"+u.version())
	return u.client.Do(req)
}

// fetchReleaseAssets 抓取 release 页面上「Assets」区域的服务端渲染片段，里面带着
// 每个安装包的下载地址。拿不到列表就无法自动安装，所以这里按失败处理。
func (u *updater) fetchReleaseAssets(base, tag string, page *url.URL) ([]releaseAsset, error) {
	resp, err := u.releasePageRequest(base + releasesAssetsSuffix + url.PathEscape(tag))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("安装包列表返回 %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseReleaseAssets(body, page, tag), nil
}

// releaseTagFromURL 从 …/releases/tag/<tag> 形式的地址里取出 tag。
func releaseTagFromURL(u *url.URL) string {
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[len(parts)-2] != "tag" {
		return ""
	}
	return parts[len(parts)-1]
}

// parseReleaseAssets 从安装包列表片段里抽出属于 tag 的安装包。名字取自下载地址的
// 最后一段，因此体积要靠下载时的响应头确认，API 给出的体积这里没有。
func parseReleaseAssets(page []byte, base *url.URL, tag string) []releaseAsset {
	prefix := "/releases/download/" + tag + "/"
	seen := make(map[string]bool)
	var assets []releaseAsset
	for _, match := range releaseAssetLinkRE.FindAllSubmatch(page, -1) {
		ref, err := url.Parse(html.UnescapeString(string(match[1])))
		if err != nil || !strings.Contains(ref.Path, prefix) {
			continue
		}
		name := path.Base(ref.Path)
		if name == "" || name == "." || name == "/" || seen[name] {
			continue
		}
		seen[name] = true
		ref.Scheme = base.Scheme
		ref.Host = base.Host
		assets = append(assets, releaseAsset{Name: name, URL: ref.String()})
	}
	return assets
}

// isRateLimitResponse 判断 403/429 是否真的由限流引起。GitHub 会在响应体的 message
// 里说明原因，代理或防火墙返回的 403 不会被误判成「请求过于频繁」。
func isRateLimitResponse(body []byte) bool {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(payload.Message), "rate limit")
}

// rememberRateLimit 根据响应头记录限流窗口。配额刚好用尽的 200 也会被记下来，
// 这样窗口结束前不会再发无效请求。
func (u *updater) rememberRateLimit(cache *updateCheckCache, resp *http.Response, now time.Time, rateLimited bool) {
	until := rateLimitDeadline(resp, now)
	if until == 0 && rateLimited {
		until = now.Add(updateRateLimitFallback).Unix()
	}
	if until > cache.LimitedUntil {
		cache.LimitedUntil = until
	}
}

// rateLimitDeadline 从响应头推算限流窗口的结束时间，没有可用的头时返回 0。
func rateLimitDeadline(resp *http.Response, now time.Time) int64 {
	if seconds, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && seconds > 0 {
		return clampRateLimitDeadline(now.Add(time.Duration(seconds)*time.Second), now)
	}
	if strings.TrimSpace(resp.Header.Get("X-RateLimit-Remaining")) != "0" {
		return 0
	}
	reset, err := strconv.ParseInt(strings.TrimSpace(resp.Header.Get("X-RateLimit-Reset")), 10, 64)
	if err != nil || reset <= 0 {
		return 0
	}
	return clampRateLimitDeadline(time.Unix(reset, 0), now)
}

// clampRateLimitDeadline 把窗口限制在一小时以内，并忽略已经过去的结束时间。
func clampRateLimitDeadline(deadline, now time.Time) int64 {
	if ceiling := now.Add(updateRateLimitCeiling); deadline.After(ceiling) {
		deadline = ceiling
	}
	if !deadline.After(now) {
		return 0
	}
	return deadline.Unix()
}

// parseRelease 把发布接口的响应体转换成内部结构。
func parseRelease(raw []byte) (releaseInfo, error) {
	var payload githubRelease
	if err := json.Unmarshal(raw, &payload); err != nil {
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

// CleanupStaleDownloads 回收上一次运行留下的东西：更新目录里只保留最近下载的那个安装包
// （用户下载完还没点安装就退出了的话，下次检查可以直接复用它，不必重新下载），并清掉
// 安装助手遗留的临时目录。
func (u *updater) CleanupStaleDownloads() {
	dir, err := u.updatesDir()
	if err != nil {
		return
	}
	u.discardDownloads(dir, newestDownload(dir))
	cleanupUpdateWorkDirs(os.TempDir(), time.Now())
}

// discardDownloads 删掉目录里除 keep 以外的安装包与下载残留。
func (u *updater) discardDownloads(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == keep {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}

// newestDownload 返回目录里最近修改的安装包名（忽略下载残留），没有时返回空串。
func newestDownload(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	name := ""
	var newest time.Time
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), downloadPartSuffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if name == "" || info.ModTime().After(newest) {
			name, newest = entry.Name(), info.ModTime()
		}
	}
	return name
}

// cleanupUpdateWorkDirs 删掉安装助手遗留的临时目录。助手脚本自己会清理，但 Windows 上
// 脚本删不掉自己所在的目录，脚本或应用被强杀时也会留下。只清理超过 updateWorkDirStaleAge
// 的目录，避免误删正在等应用退出的那一个。
func cleanupUpdateWorkDirs(tempDir string, now time.Time) {
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), updateWorkDirPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < updateWorkDirStaleAge {
			continue
		}
		_ = os.RemoveAll(filepath.Join(tempDir, entry.Name()))
	}
}

// download 把安装包下载到 ~/.veda/updates。已存在且大小一致的会被复用。
func (u *updater) download(asset releaseAsset) (string, error) {
	dir, err := u.updatesDir()
	if err != nil {
		return "", err
	}
	final := filepath.Join(dir, filepath.Base(asset.Name))
	// 目录里只留正在下载的这一个包，避免跨版本累积。
	u.discardDownloads(dir, filepath.Base(final))
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

	tmp := final + downloadPartSuffix
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
