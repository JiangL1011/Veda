package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const settingsFileName = "settings.json"

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

const (
	startupLast       = "last"
	startupLaunch     = "launch"
	docTabsSingle     = "single"
	docTabsMulti      = "multi"
	defaultMaxDocTabs = 10
	minMaxDocTabs     = 1
	maxMaxDocTabs     = 100
)

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
}

// MarkdownSettings 控制编辑区画布以及 Markdown 的渲染效果。
type MarkdownSettings struct {
	EditorWidth      string         `json:"editorWidth"`
	ShowHeadingPanel bool           `json:"showHeadingPanel"`
	Styles           MarkdownStyles `json:"styles"`
}

const (
	defaultSearchCurrentFile = "Mod+F"
	defaultSearchWorkspace   = "Mod+Shift+F"
)

// defaultCloseTab 在 macOS 上是 Cmd+W，其余平台上 Ctrl+W 会被系统和 WebView 抢走，
// 所以改用 Alt+W。
func defaultCloseTab() string {
	if runtime.GOOS == "darwin" {
		return "Mod+W"
	}
	return "Alt+W"
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
}

// SettingsBundle 是设置界面一次请求就能加载到的全部内容。
type SettingsBundle struct {
	Global          AppSettings `json:"global"`
	Workspace       AppSettings `json:"workspace"`
	WorkspaceExists bool        `json:"workspaceExists"`
}

type settingsStore struct {
	mu sync.Mutex
}

func defaultFontSans() string {
	return `-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`
}

func defaultFontMono() string {
	return `ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace`
}

func mdStyle(light, dark, bgLight, bgDark, size, font string, bold, italic bool) MarkdownStyle {
	return MarkdownStyle{
		Color:      ThemeColor{Light: light, Dark: dark},
		Background: ThemeColor{Light: bgLight, Dark: bgDark},
		FontSize:   size,
		FontFamily: font,
		Bold:       bold,
		Italic:     italic,
	}
}

// DefaultSettings 是内置的浅色/深色主题与排版预设。
func DefaultSettings() AppSettings {
	sans := defaultFontSans()
	mono := defaultFontMono()
	ink := "#1c1917"
	inkDark := "#e7e5e4"
	headDark := "#fafaf9"
	muted := "#57534e"
	mutedDark := "#a8a29e"
	subtle := "#a8a29e"
	subtleDark := "#78716c"
	link := "#1d4ed8"
	linkDark := "#93c5fd"
	line := "#e7e5e4"
	lineDark := "#44403c"
	codeBg := "#f5f5f4"
	codeBgDark := "#44403c"
	preBg := "#1c1917"
	preBgDark := "#0c0a09"
	preFg := "#f5f5f4"
	transparent := "transparent"

	return AppSettings{
		General: GeneralSettings{
			Theme:                 "system",
			Language:              "zh-CN",
			Startup:               startupLast,
			AutoReadonly:          true,
			ResourceDirectory:     "assets",
			ShowResourceDirectory: false,
			MaxDocTabs:            defaultMaxDocTabs,
			DocTabsLayout:         docTabsSingle,
			AutoCheckUpdates:      true,
		},
		Shortcuts: ShortcutSettings{
			SearchCurrentFile: defaultSearchCurrentFile,
			SearchWorkspace:   defaultSearchWorkspace,
			CloseTab:          defaultCloseTab(),
		},
		Markdown: MarkdownSettings{
			EditorWidth:      "750px",
			ShowHeadingPanel: true,
			Styles: MarkdownStyles{
				Paragraph:     mdStyle(ink, inkDark, transparent, transparent, "16px", sans, false, false),
				Heading1:      mdStyle(ink, headDark, transparent, transparent, "2rem", sans, true, false),
				Heading2:      mdStyle(ink, headDark, transparent, transparent, "1.55rem", sans, true, false),
				Heading3:      mdStyle(ink, headDark, transparent, transparent, "1.25rem", sans, true, false),
				Heading4:      mdStyle(ink, headDark, transparent, transparent, "1.05rem", sans, true, false),
				Heading5:      mdStyle(ink, headDark, transparent, transparent, "1rem", sans, true, false),
				Heading6:      mdStyle(ink, mutedDark, transparent, transparent, "0.95rem", sans, true, false),
				Strong:        mdStyle(ink, inkDark, transparent, transparent, "inherit", "inherit", true, false),
				Emphasis:      mdStyle(ink, inkDark, transparent, transparent, "inherit", "inherit", false, true),
				Strikethrough: mdStyle(muted, mutedDark, transparent, transparent, "inherit", "inherit", false, false),
				Link:          mdStyle(link, linkDark, transparent, transparent, "inherit", "inherit", false, false),
				InlineCode:    mdStyle(ink, inkDark, codeBg, codeBgDark, "0.88em", mono, false, false),
				CodeBlock:     mdStyle(preFg, inkDark, preBg, preBgDark, "13px", mono, false, false),
				Blockquote:    mdStyle(muted, mutedDark, transparent, transparent, "inherit", sans, false, false),
				UnorderedList: mdStyle(ink, inkDark, transparent, transparent, "inherit", sans, false, false),
				OrderedList:   mdStyle(ink, inkDark, transparent, transparent, "inherit", sans, false, false),
				ListItem:      mdStyle("#44403c", mutedDark, transparent, transparent, "inherit", "inherit", false, false),
				TaskList:      mdStyle(ink, inkDark, transparent, transparent, "inherit", sans, false, false),
				TaskChecked:   mdStyle(subtle, subtleDark, transparent, transparent, "inherit", "inherit", false, false),
				Table:         mdStyle(line, lineDark, transparent, transparent, "inherit", sans, false, false),
				TableHeader:   mdStyle(ink, headDark, codeBg, "#292524", "inherit", sans, true, false),
				TableCell:     mdStyle(ink, inkDark, transparent, transparent, "inherit", sans, false, false),
				Hr:            mdStyle(line, lineDark, transparent, transparent, "inherit", sans, false, false),
				Image:         mdStyle(ink, inkDark, transparent, transparent, "inherit", sans, false, false),
			},
		},
	}
}

func normalizeGeneral(s *GeneralSettings) {
	switch s.Theme {
	case "light", "dark", "system":
	default:
		s.Theme = "system"
	}
	switch s.Language {
	case "zh-CN", "en":
	default:
		s.Language = "zh-CN"
	}
	switch s.Startup {
	case startupLast, startupLaunch:
	default:
		s.Startup = startupLast
	}
	if s.MaxDocTabs < minMaxDocTabs {
		s.MaxDocTabs = minMaxDocTabs
	} else if s.MaxDocTabs > maxMaxDocTabs {
		s.MaxDocTabs = maxMaxDocTabs
	}
	switch s.DocTabsLayout {
	case docTabsSingle, docTabsMulti:
	default:
		s.DocTabsLayout = docTabsSingle
	}
	s.ResourceDirectory = normalizeResourceDirectory(s.ResourceDirectory)
}

func normalizeResourceDirectory(value string) string {
	value = filepath.Clean(strings.TrimSpace(value))
	if value == "" || value == "." || filepath.IsAbs(value) {
		return "assets"
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return "assets"
		}
	}
	return value
}

func normalizeEditorWidth(width string) string {
	if width == "" {
		return "750px"
	}
	return width
}

func normalizeShortcuts(s *ShortcutSettings) {
	if s.SearchCurrentFile == "" && s.SearchWorkspace == "" {
		// 两个都为空是合法的（用户把快捷键全解绑了）。缺失的字段在进入本函数前
		// 已经由 DefaultSettings 叠加覆盖层补齐。
	}
}

func asStringMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func mapString(m map[string]any, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

func mapBool(m map[string]any, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func mapInt(m map[string]any, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	n, ok := v.(float64)
	return int(n), ok && n == float64(int(n))
}

func applyThemeColor(base *ThemeColor, raw any) {
	m := asStringMap(raw)
	if m == nil {
		return
	}
	if v, ok := mapString(m, "light"); ok {
		base.Light = v
	}
	if v, ok := mapString(m, "dark"); ok {
		base.Dark = v
	}
}

func applyStyle(base *MarkdownStyle, raw any) {
	m := asStringMap(raw)
	if m == nil {
		return
	}
	if v, ok := m["color"]; ok {
		applyThemeColor(&base.Color, v)
	}
	if v, ok := m["background"]; ok {
		applyThemeColor(&base.Background, v)
	}
	if v, ok := mapString(m, "fontSize"); ok {
		base.FontSize = v
	}
	if v, ok := mapString(m, "fontFamily"); ok {
		base.FontFamily = v
	}
	if v, ok := mapBool(m, "bold"); ok {
		base.Bold = v
	}
	if v, ok := mapBool(m, "italic"); ok {
		base.Italic = v
	}
}

// applyShortcut 覆盖一个快捷键绑定。这里不能用 mapString，因为空字符串是明确的
// “解绑”，而不是“未设置”。
func applyShortcut(dst *string, shortcuts map[string]any, key string) {
	v, ok := shortcuts[key]
	if !ok {
		return
	}
	if s, isStr := v.(string); isStr {
		*dst = strings.TrimSpace(s)
	}
}

func applySettingsMap(base *AppSettings, raw map[string]any) {
	if raw == nil {
		return
	}
	if general := asStringMap(raw["general"]); general != nil {
		if v, ok := mapString(general, "theme"); ok {
			base.General.Theme = v
		}
		if v, ok := mapString(general, "language"); ok {
			base.General.Language = v
		}
		if v, ok := mapString(general, "startup"); ok {
			base.General.Startup = v
		}
		if v, ok := mapBool(general, "autoReadonly"); ok {
			base.General.AutoReadonly = v
		}
		if v, ok := mapString(general, "resourceDirectory"); ok {
			base.General.ResourceDirectory = v
		}
		if v, ok := mapBool(general, "showResourceDirectory"); ok {
			base.General.ShowResourceDirectory = v
		}
		if v, ok := mapInt(general, "maxDocTabs"); ok {
			base.General.MaxDocTabs = v
		}
		if v, ok := mapString(general, "docTabsLayout"); ok {
			base.General.DocTabsLayout = v
		}
		if v, ok := mapBool(general, "autoCheckUpdates"); ok {
			base.General.AutoCheckUpdates = v
		}
	}
	if markdown := asStringMap(raw["markdown"]); markdown != nil {
		if v, ok := mapString(markdown, "editorWidth"); ok {
			base.Markdown.EditorWidth = v
		}
		if v, ok := mapBool(markdown, "showHeadingPanel"); ok {
			base.Markdown.ShowHeadingPanel = v
		}
		if styles := asStringMap(markdown["styles"]); styles != nil {
			applyStyle(&base.Markdown.Styles.Paragraph, styles["paragraph"])
			applyStyle(&base.Markdown.Styles.Heading1, styles["heading1"])
			applyStyle(&base.Markdown.Styles.Heading2, styles["heading2"])
			applyStyle(&base.Markdown.Styles.Heading3, styles["heading3"])
			applyStyle(&base.Markdown.Styles.Heading4, styles["heading4"])
			applyStyle(&base.Markdown.Styles.Heading5, styles["heading5"])
			applyStyle(&base.Markdown.Styles.Heading6, styles["heading6"])
			applyStyle(&base.Markdown.Styles.Strong, styles["strong"])
			applyStyle(&base.Markdown.Styles.Emphasis, styles["emphasis"])
			applyStyle(&base.Markdown.Styles.Strikethrough, styles["strikethrough"])
			applyStyle(&base.Markdown.Styles.Link, styles["link"])
			applyStyle(&base.Markdown.Styles.InlineCode, styles["inlineCode"])
			applyStyle(&base.Markdown.Styles.CodeBlock, styles["codeBlock"])
			applyStyle(&base.Markdown.Styles.Blockquote, styles["blockquote"])
			applyStyle(&base.Markdown.Styles.UnorderedList, styles["unorderedList"])
			applyStyle(&base.Markdown.Styles.OrderedList, styles["orderedList"])
			applyStyle(&base.Markdown.Styles.ListItem, styles["listItem"])
			applyStyle(&base.Markdown.Styles.TaskList, styles["taskList"])
			applyStyle(&base.Markdown.Styles.TaskChecked, styles["taskChecked"])
			applyStyle(&base.Markdown.Styles.Table, styles["table"])
			applyStyle(&base.Markdown.Styles.TableHeader, styles["tableHeader"])
			applyStyle(&base.Markdown.Styles.TableCell, styles["tableCell"])
			applyStyle(&base.Markdown.Styles.Hr, styles["hr"])
			applyStyle(&base.Markdown.Styles.Image, styles["image"])
		}
	}
	if shortcuts := asStringMap(raw["shortcuts"]); shortcuts != nil {
		applyShortcut(&base.Shortcuts.SearchCurrentFile, shortcuts, "searchCurrentFile")
		applyShortcut(&base.Shortcuts.SearchWorkspace, shortcuts, "searchWorkspace")
		applyShortcut(&base.Shortcuts.CloseTab, shortcuts, "closeTab")
	}
	normalizeGeneral(&base.General)
	base.Markdown.EditorWidth = normalizeEditorWidth(base.Markdown.EditorWidth)
	normalizeShortcuts(&base.Shortcuts)
}

func parseSettingsJSON(raw []byte) AppSettings {
	out := DefaultSettings()
	applySettingsMap(&out, parseOverlayJSON(raw))
	return out
}

func parseOverlayJSON(raw []byte) map[string]any {
	if len(bytesTrimSpace(raw)) == 0 {
		return nil
	}
	var overlay map[string]any
	if err := json.Unmarshal(raw, &overlay); err != nil {
		return nil
	}
	if isEmptySettingsMap(overlay) {
		return nil
	}
	return overlay
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

func isEmptySettingsMap(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case map[string]any:
		if len(t) == 0 {
			return true
		}
		for _, child := range t {
			if !isEmptySettingsMap(child) {
				return false
			}
		}
		return true
	case string:
		// 空字符串是明确赋予的值（例如一个被解绑的快捷键）。
		return false
	default:
		return false
	}
}

func themeColorDiff(base, next ThemeColor) map[string]any {
	out := map[string]any{}
	if next.Light != base.Light {
		out["light"] = next.Light
	}
	if next.Dark != base.Dark {
		out["dark"] = next.Dark
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func styleDiff(base, next MarkdownStyle) map[string]any {
	out := map[string]any{}
	if c := themeColorDiff(base.Color, next.Color); c != nil {
		out["color"] = c
	}
	if c := themeColorDiff(base.Background, next.Background); c != nil {
		out["background"] = c
	}
	if next.FontSize != base.FontSize {
		out["fontSize"] = next.FontSize
	}
	if next.FontFamily != base.FontFamily {
		out["fontFamily"] = next.FontFamily
	}
	if next.Bold != base.Bold {
		out["bold"] = next.Bold
	}
	if next.Italic != base.Italic {
		out["italic"] = next.Italic
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addStyleDiff(styles map[string]any, key string, base, next MarkdownStyle) {
	if diff := styleDiff(base, next); diff != nil {
		styles[key] = diff
	}
}

func settingsDiff(base, next AppSettings) map[string]any {
	out := map[string]any{}
	general := map[string]any{}
	if next.General.Theme != base.General.Theme {
		general["theme"] = next.General.Theme
	}
	if next.General.Language != base.General.Language {
		general["language"] = next.General.Language
	}
	if next.General.AutoReadonly != base.General.AutoReadonly {
		general["autoReadonly"] = next.General.AutoReadonly
	}
	if next.General.ResourceDirectory != base.General.ResourceDirectory {
		general["resourceDirectory"] = next.General.ResourceDirectory
	}
	if next.General.ShowResourceDirectory != base.General.ShowResourceDirectory {
		general["showResourceDirectory"] = next.General.ShowResourceDirectory
	}
	if next.General.MaxDocTabs != base.General.MaxDocTabs {
		general["maxDocTabs"] = next.General.MaxDocTabs
	}
	if next.General.DocTabsLayout != base.General.DocTabsLayout {
		general["docTabsLayout"] = next.General.DocTabsLayout
	}
	// Startup 与 AutoCheckUpdates 仅对全局设置生效，不会写进工作区覆盖层。
	if len(general) > 0 {
		out["general"] = general
	}

	markdown := map[string]any{}
	if next.Markdown.EditorWidth != base.Markdown.EditorWidth {
		markdown["editorWidth"] = next.Markdown.EditorWidth
	}
	if next.Markdown.ShowHeadingPanel != base.Markdown.ShowHeadingPanel {
		markdown["showHeadingPanel"] = next.Markdown.ShowHeadingPanel
	}
	styles := map[string]any{}
	addStyleDiff(styles, "paragraph", base.Markdown.Styles.Paragraph, next.Markdown.Styles.Paragraph)
	addStyleDiff(styles, "heading1", base.Markdown.Styles.Heading1, next.Markdown.Styles.Heading1)
	addStyleDiff(styles, "heading2", base.Markdown.Styles.Heading2, next.Markdown.Styles.Heading2)
	addStyleDiff(styles, "heading3", base.Markdown.Styles.Heading3, next.Markdown.Styles.Heading3)
	addStyleDiff(styles, "heading4", base.Markdown.Styles.Heading4, next.Markdown.Styles.Heading4)
	addStyleDiff(styles, "heading5", base.Markdown.Styles.Heading5, next.Markdown.Styles.Heading5)
	addStyleDiff(styles, "heading6", base.Markdown.Styles.Heading6, next.Markdown.Styles.Heading6)
	addStyleDiff(styles, "strong", base.Markdown.Styles.Strong, next.Markdown.Styles.Strong)
	addStyleDiff(styles, "emphasis", base.Markdown.Styles.Emphasis, next.Markdown.Styles.Emphasis)
	addStyleDiff(styles, "strikethrough", base.Markdown.Styles.Strikethrough, next.Markdown.Styles.Strikethrough)
	addStyleDiff(styles, "link", base.Markdown.Styles.Link, next.Markdown.Styles.Link)
	addStyleDiff(styles, "inlineCode", base.Markdown.Styles.InlineCode, next.Markdown.Styles.InlineCode)
	addStyleDiff(styles, "codeBlock", base.Markdown.Styles.CodeBlock, next.Markdown.Styles.CodeBlock)
	addStyleDiff(styles, "blockquote", base.Markdown.Styles.Blockquote, next.Markdown.Styles.Blockquote)
	addStyleDiff(styles, "unorderedList", base.Markdown.Styles.UnorderedList, next.Markdown.Styles.UnorderedList)
	addStyleDiff(styles, "orderedList", base.Markdown.Styles.OrderedList, next.Markdown.Styles.OrderedList)
	addStyleDiff(styles, "listItem", base.Markdown.Styles.ListItem, next.Markdown.Styles.ListItem)
	addStyleDiff(styles, "taskList", base.Markdown.Styles.TaskList, next.Markdown.Styles.TaskList)
	addStyleDiff(styles, "taskChecked", base.Markdown.Styles.TaskChecked, next.Markdown.Styles.TaskChecked)
	addStyleDiff(styles, "table", base.Markdown.Styles.Table, next.Markdown.Styles.Table)
	addStyleDiff(styles, "tableHeader", base.Markdown.Styles.TableHeader, next.Markdown.Styles.TableHeader)
	addStyleDiff(styles, "tableCell", base.Markdown.Styles.TableCell, next.Markdown.Styles.TableCell)
	addStyleDiff(styles, "hr", base.Markdown.Styles.Hr, next.Markdown.Styles.Hr)
	addStyleDiff(styles, "image", base.Markdown.Styles.Image, next.Markdown.Styles.Image)
	if len(styles) > 0 {
		markdown["styles"] = styles
	}
	if len(markdown) > 0 {
		out["markdown"] = markdown
	}

	shortcuts := map[string]any{}
	if next.Shortcuts.SearchCurrentFile != base.Shortcuts.SearchCurrentFile {
		shortcuts["searchCurrentFile"] = next.Shortcuts.SearchCurrentFile
	}
	if next.Shortcuts.SearchWorkspace != base.Shortcuts.SearchWorkspace {
		shortcuts["searchWorkspace"] = next.Shortcuts.SearchWorkspace
	}
	if next.Shortcuts.CloseTab != base.Shortcuts.CloseTab {
		shortcuts["closeTab"] = next.Shortcuts.CloseTab
	}
	if len(shortcuts) > 0 {
		out["shortcuts"] = shortcuts
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func globalSettingsPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, settingsFileName), nil
}

func workspaceSettingsPath(workspacePath string) (string, error) {
	return workspaceFile(workspacePath, settingsFileName)
}

func readSettingsFile(path string) (AppSettings, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), false, nil
		}
		return DefaultSettings(), false, err
	}
	overlay := parseOverlayJSON(raw)
	return parseSettingsJSON(raw), overlay != nil, nil
}

func readOverlayFile(path string) (map[string]any, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return parseOverlayJSON(raw), true, nil
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeSettingsFile(path string, settings AppSettings) error {
	normalizeGeneral(&settings.General)
	settings.Markdown.EditorWidth = normalizeEditorWidth(settings.Markdown.EditorWidth)
	return writeJSONFile(path, settings)
}

func removeWorkspaceSettings(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
	return nil
}

func (s *settingsStore) ensureGlobal() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ensureGlobalLocked()
}

func (s *settingsStore) ensureGlobalLocked() error {
	path, err := globalSettingsPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeSettingsFile(path, DefaultSettings())
}

func (s *settingsStore) loadBundle(workspacePath string) (*SettingsBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadBundleLocked(workspacePath)
}

func (s *settingsStore) loadBundleLocked(workspacePath string) (*SettingsBundle, error) {
	if err := s.ensureGlobalLocked(); err != nil {
		return nil, err
	}
	globalPath, err := globalSettingsPath()
	if err != nil {
		return nil, err
	}
	global, _, err := readSettingsFile(globalPath)
	if err != nil {
		return nil, err
	}

	bundle := &SettingsBundle{
		Global:    global,
		Workspace: global,
	}
	if workspacePath == "" {
		return bundle, nil
	}
	wsPath, err := workspaceSettingsPath(workspacePath)
	if err != nil {
		return nil, err
	}
	overlay, exists, err := readOverlayFile(wsPath)
	if err != nil {
		return nil, err
	}
	if !exists {
		return bundle, nil
	}
	workspace := global
	applySettingsMap(&workspace, overlay)
	workspace.General.Startup = global.General.Startup
	workspace.General.AutoCheckUpdates = global.General.AutoCheckUpdates
	bundle.Workspace = workspace
	bundle.WorkspaceExists = true
	return bundle, nil
}

// autoCheckUpdates 读取全局设置里的自动检查更新开关。读取失败时按开启处理，
// 与默认设置保持一致。
func (s *settingsStore) autoCheckUpdates() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureGlobalLocked(); err != nil {
		return true
	}
	path, err := globalSettingsPath()
	if err != nil {
		return true
	}
	settings, _, err := readSettingsFile(path)
	if err != nil {
		return true
	}
	return settings.General.AutoCheckUpdates
}

func (s *settingsStore) shouldRestoreOnStartup() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureGlobalLocked(); err != nil {
		return true
	}
	path, err := globalSettingsPath()
	if err != nil {
		return true
	}
	settings, _, err := readSettingsFile(path)
	if err != nil {
		return true
	}
	return settings.General.Startup != startupLaunch
}

func (s *settingsStore) save(scope, workspacePath string, settings AppSettings) (*SettingsBundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureGlobalLocked(); err != nil {
		return nil, err
	}

	switch scope {
	case "global":
		path, err := globalSettingsPath()
		if err != nil {
			return nil, err
		}
		if err := writeSettingsFile(path, settings); err != nil {
			return nil, err
		}
	case "workspace":
		if workspacePath == "" {
			return nil, errors.New("当前窗口不是工作区，只能修改全局设置")
		}
		path, err := workspaceSettingsPath(workspacePath)
		if err != nil {
			return nil, err
		}
		globalPath, err := globalSettingsPath()
		if err != nil {
			return nil, err
		}
		global, _, err := readSettingsFile(globalPath)
		if err != nil {
			return nil, err
		}
		settings.General.Startup = global.General.Startup
		settings.General.AutoCheckUpdates = global.General.AutoCheckUpdates
		overlay := settingsDiff(global, settings)
		if overlay == nil {
			overlay = map[string]any{}
		}
		if err := writeJSONFile(path, overlay); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("无效的设置范围")
	}
	return s.loadBundleLocked(workspacePath)
}
