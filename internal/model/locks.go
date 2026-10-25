package model

// FileLockState 是文档持久化的锁定状态（只读还是可编辑）。
type FileLockState struct {
	Found   bool `json:"found"`
	Editing bool `json:"editing"`
}
