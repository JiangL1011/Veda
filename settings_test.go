package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSettingsJSONKeepsDefaultBold(t *testing.T) {
	raw := []byte(`{
  "general": {
    "theme": "dark",
    "language": "en"
  },
  "markdown": {
    "editorWidth": "80%",
    "styles": {
      "heading1": {
        "color": {
          "light": "#ff0000"
        }
      }
    }
  }
}`)
	got := parseSettingsJSON(raw)
	if got.General.Theme != "dark" || got.General.Language != "en" {
		t.Fatalf("general: %+v", got.General)
	}
	if got.General.Startup != startupLast {
		t.Fatalf("missing startup should default to last, got %s", got.General.Startup)
	}
	if !got.General.AutoReadonly {
		t.Fatal("missing autoReadonly should default to enabled")
	}
	if got.General.MaxDocTabs != defaultMaxDocTabs || got.General.DocTabsLayout != docTabsSingle {
		t.Fatalf("missing tab settings should use defaults, got %+v", got.General)
	}
	if got.Markdown.EditorWidth != "80%" {
		t.Fatalf("width: %s", got.Markdown.EditorWidth)
	}
	if !got.Markdown.ShowHeadingPanel {
		t.Fatal("missing showHeadingPanel should default to enabled")
	}
	if got.Markdown.Styles.Heading1.Color.Light != "#ff0000" {
		t.Fatalf("heading color: %+v", got.Markdown.Styles.Heading1.Color)
	}
	if !got.Markdown.Styles.Heading1.Bold {
		t.Fatal("partial heading overlay should keep default bold")
	}
	if got.Markdown.Styles.Heading1.FontSize != "2rem" {
		t.Fatalf("heading size: %s", got.Markdown.Styles.Heading1.FontSize)
	}
	if got.Markdown.Styles.Paragraph.FontSize != "16px" {
		t.Fatalf("paragraph should stay default, got %s", got.Markdown.Styles.Paragraph.FontSize)
	}
}

func TestEnsureGlobalCreatesDefaultFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &settingsStore{}
	if err := store.ensureGlobal(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".veda", "settings.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"theme": "system"`) {
		t.Fatalf("expected default theme to follow system, got %s", raw)
	}
	if !strings.Contains(string(raw), `"startup": "last"`) {
		t.Fatalf("expected default startup to restore last workspace, got %s", raw)
	}
	if !strings.Contains(string(raw), `"autoReadonly": true`) {
		t.Fatalf("expected default autoReadonly to be enabled, got %s", raw)
	}
	if !strings.Contains(string(raw), `"showHeadingPanel": true`) {
		t.Fatalf("expected heading panel to be shown by default, got %s", raw)
	}
	if !strings.Contains(string(raw), `"searchCurrentFile": "Mod+F"`) {
		t.Fatalf("expected default file search shortcut, got %s", raw)
	}
	if !strings.Contains(string(raw), `"searchWorkspace": "Mod+Shift+F"`) {
		t.Fatalf("expected default workspace search shortcut, got %s", raw)
	}
	again, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ensureGlobal(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(second) {
		t.Fatal("ensureGlobal should not rewrite an existing file")
	}
}

func TestSettingsStoreRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &settingsStore{}
	settings := DefaultSettings()
	settings.General.Theme = "dark"
	settings.General.Language = "en"
	settings.General.MaxDocTabs = 24
	settings.General.DocTabsLayout = docTabsMulti
	settings.Markdown.EditorWidth = "90%"
	settings.Markdown.ShowHeadingPanel = false
	settings.Markdown.Styles.Heading1.Color.Light = "#111111"

	if _, err := store.save("global", "", settings); err != nil {
		t.Fatal(err)
	}

	bundle, err := store.loadBundle("")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.WorkspaceExists {
		t.Fatal("no workspace expected")
	}
	if bundle.Global.General.Theme != "dark" || bundle.Global.General.Language != "en" {
		t.Fatalf("loaded global: %+v", bundle.Global.General)
	}
	if bundle.Global.General.MaxDocTabs != 24 || bundle.Global.General.DocTabsLayout != docTabsMulti {
		t.Fatalf("tab settings should round-trip: %+v", bundle.Global.General)
	}
	if bundle.Global.Markdown.EditorWidth != "90%" {
		t.Fatalf("width: %s", bundle.Global.Markdown.EditorWidth)
	}
	if bundle.Global.Markdown.ShowHeadingPanel {
		t.Fatal("heading panel preference should round-trip")
	}
	if bundle.Global.Markdown.Styles.Heading1.Color.Light != "#111111" {
		t.Fatalf("heading: %+v", bundle.Global.Markdown.Styles.Heading1.Color)
	}
	if !bundle.Global.Markdown.Styles.Heading1.Bold {
		t.Fatal("bold should remain")
	}
}

func TestWorkspaceWritesOnlyDiffs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &settingsStore{}
	global := DefaultSettings()
	global.General.Theme = "dark"
	if _, err := store.save("global", "", global); err != nil {
		t.Fatal(err)
	}

	ws := t.TempDir()
	same := DefaultSettings()
	same.General.Theme = "dark"
	bundle, err := store.save("workspace", ws, same)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.WorkspaceExists {
		t.Fatal("workspace save should create settings.json even when values match global")
	}
	raw := mustWorkspaceFileBytes(t, ws, "settings.json")
	var emptyOverlay map[string]any
	if err := json.Unmarshal(raw, &emptyOverlay); err != nil {
		t.Fatal(err)
	}
	if !isEmptySettingsMap(emptyOverlay) {
		t.Fatalf("matching values should be omitted, got %s", raw)
	}
	if bundle.Workspace.General.Theme != "dark" {
		t.Fatalf("workspace should inherit global, got %s", bundle.Workspace.General.Theme)
	}
	assertNoWorkspaceDotDir(t, ws)

	changed := same
	changed.General.Language = "en"
	changed.General.MaxDocTabs = 18
	changed.General.DocTabsLayout = docTabsMulti
	changed.Markdown.EditorWidth = "80%"
	changed.Markdown.ShowHeadingPanel = false
	bundle, err = store.save("workspace", ws, changed)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.WorkspaceExists {
		t.Fatal("expected workspace overlay")
	}
	raw = mustWorkspaceFileBytes(t, ws, "settings.json")
	var overlay map[string]any
	if err := json.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	if _, ok := overlay["general"].(map[string]any)["theme"]; ok {
		t.Fatalf("theme matches global and should be omitted: %s", raw)
	}
	if overlay["general"].(map[string]any)["language"] != "en" {
		t.Fatalf("language diff: %s", raw)
	}
	if overlay["general"].(map[string]any)["maxDocTabs"] != float64(18) {
		t.Fatalf("maxDocTabs diff: %s", raw)
	}
	if overlay["general"].(map[string]any)["docTabsLayout"] != docTabsMulti {
		t.Fatalf("docTabsLayout diff: %s", raw)
	}
	if overlay["markdown"].(map[string]any)["editorWidth"] != "80%" {
		t.Fatalf("width diff: %s", raw)
	}
	if overlay["markdown"].(map[string]any)["showHeadingPanel"] != false {
		t.Fatalf("heading panel diff: %s", raw)
	}
	if bundle.Workspace.General.Theme != "dark" || bundle.Workspace.General.Language != "en" {
		t.Fatalf("effective workspace: %+v", bundle.Workspace.General)
	}

	reverted := changed
	reverted.General.Language = "zh-CN"
	bundle, err = store.save("workspace", ws, reverted)
	if err != nil {
		t.Fatal(err)
	}
	raw = mustWorkspaceFileBytes(t, ws, "settings.json")
	overlay = map[string]any{}
	if err := json.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	if general, ok := overlay["general"].(map[string]any); ok {
		if _, hasLang := general["language"]; hasLang {
			t.Fatalf("matching language should be removed: %s", raw)
		}
	}
	if overlay["markdown"].(map[string]any)["editorWidth"] != "80%" {
		t.Fatalf("remaining diff should stay: %s", raw)
	}

	empty := t.TempDir()
	emptySettings, err := workspaceSettingsPath(empty)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(emptySettings, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err = store.loadBundle(empty)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.WorkspaceExists || bundle.Workspace.General.Theme != "dark" {
		t.Fatalf("empty overlay should fall back to global: exists=%v theme=%s", bundle.WorkspaceExists, bundle.Workspace.General.Theme)
	}
}

func TestStartupIsGlobalOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &settingsStore{}
	global := DefaultSettings()
	global.General.Startup = startupLaunch
	if _, err := store.save("global", "", global); err != nil {
		t.Fatal(err)
	}
	if store.shouldRestoreOnStartup() {
		t.Fatal("launch startup should not restore the last workspace")
	}

	ws := t.TempDir()
	changed := DefaultSettings()
	changed.General.Startup = startupLast
	changed.General.Language = "en"
	bundle, err := store.save("workspace", ws, changed)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Workspace.General.Startup != startupLaunch {
		t.Fatalf("workspace should keep global startup, got %s", bundle.Workspace.General.Startup)
	}
	raw := mustWorkspaceFileBytes(t, ws, "settings.json")
	var overlay map[string]any
	if err := json.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	if general, ok := overlay["general"].(map[string]any); ok {
		if _, hasStartup := general["startup"]; hasStartup {
			t.Fatalf("startup must not be written to workspace settings: %s", raw)
		}
	}

	overlayPath, err := workspaceSettingsPath(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlayPath, []byte(`{"general":{"startup":"last","language":"en"}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle, err = store.loadBundle(ws)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Workspace.General.Startup != startupLaunch {
		t.Fatalf("workspace overlay must not override startup, got %s", bundle.Workspace.General.Startup)
	}
	if bundle.Workspace.General.Language != "en" {
		t.Fatalf("other workspace general fields should still apply, got %s", bundle.Workspace.General.Language)
	}

	global.General.Startup = startupLast
	if _, err := store.save("global", "", global); err != nil {
		t.Fatal(err)
	}
	if !store.shouldRestoreOnStartup() {
		t.Fatal("last startup should restore the previous workspace")
	}
}

func TestAutoReadonlyDefaultsAndWorkspaceOverlay(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &settingsStore{}
	global := DefaultSettings()
	if !global.General.AutoReadonly {
		t.Fatal("default autoReadonly should be enabled")
	}
	if _, err := store.save("global", "", global); err != nil {
		t.Fatal(err)
	}

	ws := t.TempDir()
	changed := DefaultSettings()
	changed.General.AutoReadonly = false
	bundle, err := store.save("workspace", ws, changed)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.Global.General.AutoReadonly {
		t.Fatal("global autoReadonly should stay enabled")
	}
	if bundle.Workspace.General.AutoReadonly {
		t.Fatal("workspace autoReadonly should be disabled")
	}
	raw := mustWorkspaceFileBytes(t, ws, "settings.json")
	var overlay map[string]any
	if err := json.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	general, _ := overlay["general"].(map[string]any)
	if general["autoReadonly"] != false {
		t.Fatalf("workspace overlay should record autoReadonly=false, got %s", raw)
	}

	got := parseSettingsJSON([]byte(`{"general":{"autoReadonly":false}}`))
	if got.General.AutoReadonly {
		t.Fatal("explicit autoReadonly false should stay disabled")
	}
}

func TestShortcutOverlayAndUnbind(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	store := &settingsStore{}
	global := DefaultSettings()
	if global.Shortcuts.SearchCurrentFile != defaultSearchCurrentFile {
		t.Fatalf("default file search: %s", global.Shortcuts.SearchCurrentFile)
	}
	if _, err := store.save("global", "", global); err != nil {
		t.Fatal(err)
	}

	got := parseSettingsJSON([]byte(`{"shortcuts":{"searchCurrentFile":"Alt+F"}}`))
	if got.Shortcuts.SearchCurrentFile != "Alt+F" {
		t.Fatalf("overlay file search: %s", got.Shortcuts.SearchCurrentFile)
	}
	if got.Shortcuts.SearchWorkspace != defaultSearchWorkspace {
		t.Fatalf("missing workspace shortcut should keep default, got %s", got.Shortcuts.SearchWorkspace)
	}

	ws := t.TempDir()
	changed := DefaultSettings()
	changed.Shortcuts.SearchCurrentFile = "Alt+F"
	changed.Shortcuts.SearchWorkspace = ""
	bundle, err := store.save("workspace", ws, changed)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Workspace.Shortcuts.SearchCurrentFile != "Alt+F" {
		t.Fatalf("workspace file search: %s", bundle.Workspace.Shortcuts.SearchCurrentFile)
	}
	if bundle.Workspace.Shortcuts.SearchWorkspace != "" {
		t.Fatal("empty workspace shortcut should stay unbound")
	}
	if bundle.Global.Shortcuts.SearchWorkspace != defaultSearchWorkspace {
		t.Fatal("global workspace shortcut should stay default")
	}
	raw := mustWorkspaceFileBytes(t, ws, "settings.json")
	var overlay map[string]any
	if err := json.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	shortcuts, _ := overlay["shortcuts"].(map[string]any)
	if shortcuts["searchCurrentFile"] != "Alt+F" {
		t.Fatalf("expected file search diff, got %s", raw)
	}
	if shortcuts["searchWorkspace"] != "" {
		t.Fatalf("unbound shortcut must be written, got %s", raw)
	}
	if _, ok := shortcuts["closeTab"]; ok {
		t.Fatalf("unchanged shortcut must stay out of the overlay, got %s", raw)
	}
}

func TestCloseTabShortcut(t *testing.T) {
	if got := DefaultSettings().Shortcuts.CloseTab; got != defaultCloseTab() {
		t.Fatalf("default close tab: %s", got)
	}

	got := parseSettingsJSON([]byte(`{"shortcuts":{"closeTab":"Ctrl+Shift+W"}}`))
	if got.Shortcuts.CloseTab != "Ctrl+Shift+W" {
		t.Fatalf("overlay close tab: %s", got.Shortcuts.CloseTab)
	}
	if got.Shortcuts.SearchCurrentFile != defaultSearchCurrentFile {
		t.Fatalf("missing file search shortcut should keep default, got %s", got.Shortcuts.SearchCurrentFile)
	}

	unbound := parseSettingsJSON([]byte(`{"shortcuts":{"closeTab":""}}`))
	if unbound.Shortcuts.CloseTab != "" {
		t.Fatalf("empty close tab shortcut should stay unbound, got %s", unbound.Shortcuts.CloseTab)
	}
}

func TestSaveWorkspaceRequiresDirectory(t *testing.T) {
	store := &settingsStore{}
	if _, err := store.save("workspace", "", DefaultSettings()); err == nil {
		t.Fatal("expected error for empty workspace")
	}
	if _, err := store.save("nope", "", DefaultSettings()); err == nil {
		t.Fatal("expected invalid scope")
	}
}
