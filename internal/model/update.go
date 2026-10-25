package model

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
