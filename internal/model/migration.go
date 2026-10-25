package model

// DataMigrationState 描述启动时把旧数据迁移到 file-index.json 的进度。
// 迁移期间前端会锁住界面，只显示「新版本数据迁移中」。
type DataMigrationState struct {
	Required  bool   `json:"required"`
	Running   bool   `json:"running"`
	Done      bool   `json:"done"`
	Error     string `json:"error"`
	Total     int    `json:"total"`
	Processed int    `json:"processed"`
}
