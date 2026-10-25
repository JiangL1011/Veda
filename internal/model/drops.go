package model

// 拖拽文件进工作区时的处理方式。
const (
	DragDropAsk  = "ask"
	DragDropCopy = "copy"
	DragDropMove = "move"
)

// FileDropOutcome 是一次拖拽导入的结果：落盘的文件，以及失败项的原因。
type FileDropOutcome struct {
	DestDir   string      `json:"destDir"`
	Workspace string      `json:"workspace"`
	Action    string      `json:"action"`
	Imported  []FileEntry `json:"imported"`
	Errors    []string    `json:"errors"`
}
