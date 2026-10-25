package model

// Session 持久化在 ~/.veda/session.json 中。
type Session struct {
	Last *OpenTarget `json:"last,omitempty"`
}

// OpenTarget 是用户打开的文件或目录。
type OpenTarget struct {
	Kind string `json:"kind"` // "file" | "directory"
	Path string `json:"path"`
	File string `json:"file,omitempty"`
}
