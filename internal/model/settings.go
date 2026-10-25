package model

// ThemeColor 是 Markdown 样式使用的浅色/深色配色对。
type ThemeColor struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// MarkdownStyle 是单个 Markdown 元素的渲染样式，可由用户修改。
type MarkdownStyle struct {
	Color      ThemeColor `json:"color"`
	Background ThemeColor `json:"background"`
	FontSize   string     `json:"fontSize"`
	FontFamily string     `json:"fontFamily"`
	Bold       bool       `json:"bold"`
	Italic     bool       `json:"italic"`
}

// MarkdownStyles 列出编辑器能渲染的全部 Markdown 元素。
type MarkdownStyles struct {
	Paragraph     MarkdownStyle `json:"paragraph"`
	Heading1      MarkdownStyle `json:"heading1"`
	Heading2      MarkdownStyle `json:"heading2"`
	Heading3      MarkdownStyle `json:"heading3"`
	Heading4      MarkdownStyle `json:"heading4"`
	Heading5      MarkdownStyle `json:"heading5"`
	Heading6      MarkdownStyle `json:"heading6"`
	Strong        MarkdownStyle `json:"strong"`
	Emphasis      MarkdownStyle `json:"emphasis"`
	Strikethrough MarkdownStyle `json:"strikethrough"`
	Link          MarkdownStyle `json:"link"`
	InlineCode    MarkdownStyle `json:"inlineCode"`
	CodeBlock     MarkdownStyle `json:"codeBlock"`
	Blockquote    MarkdownStyle `json:"blockquote"`
	UnorderedList MarkdownStyle `json:"unorderedList"`
	OrderedList   MarkdownStyle `json:"orderedList"`
	ListItem      MarkdownStyle `json:"listItem"`
	TaskList      MarkdownStyle `json:"taskList"`
	TaskChecked   MarkdownStyle `json:"taskChecked"`
	Table         MarkdownStyle `json:"table"`
	TableHeader   MarkdownStyle `json:"tableHeader"`
	TableCell     MarkdownStyle `json:"tableCell"`
	Hr            MarkdownStyle `json:"hr"`
	Image         MarkdownStyle `json:"image"`
}

// GeneralSettings 是应用级别的界面偏好设置。
type GeneralSettings struct {
	Theme    string `json:"theme"`
	Language string `json:"language"`
	// Startup 仅对全局设置生效："last" 恢复上次的工作区，"launch" 打开启动页。
	Startup string `json:"startup"`
	// AutoReadonly 为 true 时文档以锁定状态打开；为 false 时每个文件的锁定状态单独持久化。
	AutoReadonly bool `json:"autoReadonly"`
	// ResourceDirectory 相对于工作区根目录；单文件窗口下则相对于 ~/.veda。
	ResourceDirectory string `json:"resourceDirectory"`
	// ShowResourceDirectory 控制工作区目录树是否显示资源目录。
	ShowResourceDirectory bool `json:"showResourceDirectory"`
	// MaxDocTabs 是每个窗口最多保留的文档标签页数量。
	MaxDocTabs int `json:"maxDocTabs"`
	// DocTabsLayout 控制标签页单行滚动或多行换行显示。
	DocTabsLayout string `json:"docTabsLayout"`
	// AutoCheckUpdates 控制是否在启动时以及之后每 6 小时自动检查更新。
	// 和 Startup 一样只存在于全局设置里。
	AutoCheckUpdates bool `json:"autoCheckUpdates"`
	// DragDropMode 决定拖拽进来的文件如何进入工作区："ask" 每次询问，
	// "copy" 直接复制，"move" 直接移动。全局与工作区都可设置。
	DragDropMode string `json:"dragDropMode"`
}

// MarkdownSettings 控制编辑区画布以及 Markdown 的渲染效果。
type MarkdownSettings struct {
	EditorWidth      string         `json:"editorWidth"`
	ShowHeadingPanel bool           `json:"showHeadingPanel"`
	Styles           MarkdownStyles `json:"styles"`
}

// HtmlSettings 控制 HTML 文件的打开方式。
type HtmlSettings struct {
	// DefaultView 是 HTML 文件默认的打开方式："source" 显示源码，"preview" 渲染预览。
	DefaultView string `json:"defaultView"`
}

// ShortcutSettings 保存可配置的命令快捷键。
// 空字符串表示该命令未绑定快捷键；JSON 中缺失的字段则沿用默认值。
type ShortcutSettings struct {
	SearchCurrentFile string `json:"searchCurrentFile"`
	SearchWorkspace   string `json:"searchWorkspace"`
	CloseTab          string `json:"closeTab"`
}

// AppSettings 是持久化后的设置文档。
type AppSettings struct {
	General   GeneralSettings  `json:"general"`
	Markdown  MarkdownSettings `json:"markdown"`
	Shortcuts ShortcutSettings `json:"shortcuts"`
	Html      HtmlSettings     `json:"html"`
}

// SettingsBundle 是设置界面一次请求就能加载到的全部内容。
type SettingsBundle struct {
	Global          AppSettings `json:"global"`
	Workspace       AppSettings `json:"workspace"`
	WorkspaceExists bool        `json:"workspaceExists"`
}
