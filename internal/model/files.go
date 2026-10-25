// Package model 定义在 Go 各层之间传递、并暴露给前端的共享数据结构。
// 这里的类型只描述数据形状，不包含业务逻辑。
package model

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

// ImportedAsset 描述已保存的图片，以及要插入到 Markdown 里的路径。
type ImportedAsset struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	MarkdownPath string `json:"markdownPath"`
}
