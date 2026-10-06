import type { AppSettings, MarkdownStyle, MarkdownStyles, ShortcutSettings, ThemeColor } from "../../bindings/veda/models";
import { DEFAULT_SEARCH_CURRENT_FILE, DEFAULT_SEARCH_WORKSPACE, defaultCloseTab } from "./shortcuts";

export type { AppSettings, MarkdownStyle, MarkdownStyles, ShortcutSettings, ThemeColor };
export type ThemeMode = "light" | "dark" | "system";
export type Language = "zh-CN" | "en";
export type StartupMode = "last" | "launch";
export type DocTabsLayout = "single" | "multi";
export type SettingsScope = "global" | "workspace";
export type StyleKey = keyof MarkdownStyles;

export const STYLE_KEYS: StyleKey[] = [
  "paragraph",
  "heading1",
  "heading2",
  "heading3",
  "heading4",
  "heading5",
  "heading6",
  "strong",
  "emphasis",
  "strikethrough",
  "link",
  "inlineCode",
  "codeBlock",
  "blockquote",
  "unorderedList",
  "orderedList",
  "listItem",
  "taskList",
  "taskChecked",
  "table",
  "tableHeader",
  "tableCell",
  "hr",
  "image",
];

const FONT_SANS = `-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`;
const FONT_MONO = `ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace`;

function style(
  light: string,
  dark: string,
  bgLight: string,
  bgDark: string,
  fontSize: string,
  fontFamily: string,
  bold: boolean,
  italic: boolean,
): MarkdownStyle {
  return {
    color: { light, dark },
    background: { light: bgLight, dark: bgDark },
    fontSize,
    fontFamily,
    bold,
    italic,
  };
}

export function defaultSettings(): AppSettings {
  const ink = "#1c1917";
  const inkDark = "#e7e5e4";
  const headDark = "#fafaf9";
  const muted = "#57534e";
  const mutedDark = "#a8a29e";
  const subtle = "#a8a29e";
  const subtleDark = "#78716c";
  const transparent = "transparent";
  return {
    general: {
      theme: "system",
      language: "zh-CN",
      startup: "last",
      autoReadonly: true,
      resourceDirectory: "assets",
      showResourceDirectory: false,
      maxDocTabs: 10,
      docTabsLayout: "single",
      autoCheckUpdates: true,
    },
    shortcuts: {
      searchCurrentFile: DEFAULT_SEARCH_CURRENT_FILE,
      searchWorkspace: DEFAULT_SEARCH_WORKSPACE,
      closeTab: defaultCloseTab(),
    },
    markdown: {
      editorWidth: "750px",
      showHeadingPanel: true,
      styles: {
        paragraph: style(ink, inkDark, transparent, transparent, "16px", FONT_SANS, false, false),
        heading1: style(ink, headDark, transparent, transparent, "2rem", FONT_SANS, true, false),
        heading2: style(ink, headDark, transparent, transparent, "1.55rem", FONT_SANS, true, false),
        heading3: style(ink, headDark, transparent, transparent, "1.25rem", FONT_SANS, true, false),
        heading4: style(ink, headDark, transparent, transparent, "1.05rem", FONT_SANS, true, false),
        heading5: style(ink, headDark, transparent, transparent, "1rem", FONT_SANS, true, false),
        heading6: style(ink, mutedDark, transparent, transparent, "0.95rem", FONT_SANS, true, false),
        strong: style(ink, inkDark, transparent, transparent, "inherit", "inherit", true, false),
        emphasis: style(ink, inkDark, transparent, transparent, "inherit", "inherit", false, true),
        strikethrough: style(muted, mutedDark, transparent, transparent, "inherit", "inherit", false, false),
        link: style("#1d4ed8", "#93c5fd", transparent, transparent, "inherit", "inherit", false, false),
        inlineCode: style(ink, inkDark, "#f5f5f4", "#44403c", "0.88em", FONT_MONO, false, false),
        codeBlock: style("#f5f5f4", inkDark, "#1c1917", "#0c0a09", "13px", FONT_MONO, false, false),
        blockquote: style(muted, mutedDark, transparent, transparent, "inherit", FONT_SANS, false, false),
        unorderedList: style(ink, inkDark, transparent, transparent, "inherit", FONT_SANS, false, false),
        orderedList: style(ink, inkDark, transparent, transparent, "inherit", FONT_SANS, false, false),
        listItem: style("#44403c", mutedDark, transparent, transparent, "inherit", "inherit", false, false),
        taskList: style(ink, inkDark, transparent, transparent, "inherit", FONT_SANS, false, false),
        taskChecked: style(subtle, subtleDark, transparent, transparent, "inherit", "inherit", false, false),
        table: style("#e7e5e4", "#44403c", transparent, transparent, "inherit", FONT_SANS, false, false),
        tableHeader: style(ink, headDark, "#f5f5f4", "#292524", "inherit", FONT_SANS, true, false),
        tableCell: style(ink, inkDark, transparent, transparent, "inherit", FONT_SANS, false, false),
        hr: style("#e7e5e4", "#44403c", transparent, transparent, "inherit", FONT_SANS, false, false),
        image: style(ink, inkDark, transparent, transparent, "inherit", FONT_SANS, false, false),
      },
    },
  };
}

export function cloneSettings(value: AppSettings): AppSettings {
  return structuredClone(value);
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object" ? (value as Record<string, unknown>) : null;
}

function pickRecord(value: Record<string, unknown>, ...keys: string[]) {
  for (const key of keys) {
    const found = asRecord(value[key]);
    if (found) {
      return found;
    }
  }
  return null;
}

function applyStyleOverlay(base: MarkdownStyle, raw: unknown): MarkdownStyle {
  const rec = asRecord(raw);
  if (!rec) {
    return base;
  }
  const color = asRecord(rec.color) ?? asRecord(rec.Color);
  const background = asRecord(rec.background) ?? asRecord(rec.Background);
  const fontSize = rec.fontSize ?? rec.FontSize;
  const fontFamily = rec.fontFamily ?? rec.FontFamily;
  const bold = rec.bold ?? rec.Bold;
  const italic = rec.italic ?? rec.Italic;
  return {
    color: {
      light: typeof color?.light === "string" ? color.light : typeof color?.Light === "string" ? color.Light : base.color.light,
      dark: typeof color?.dark === "string" ? color.dark : typeof color?.Dark === "string" ? color.Dark : base.color.dark,
    },
    background: {
      light:
        typeof background?.light === "string"
          ? background.light
          : typeof background?.Light === "string"
            ? background.Light
            : base.background.light,
      dark:
        typeof background?.dark === "string"
          ? background.dark
          : typeof background?.Dark === "string"
            ? background.Dark
            : base.background.dark,
    },
    fontSize: typeof fontSize === "string" && fontSize ? fontSize : base.fontSize,
    fontFamily: typeof fontFamily === "string" && fontFamily ? fontFamily : base.fontFamily,
    bold: typeof bold === "boolean" ? bold : base.bold,
    italic: typeof italic === "boolean" ? italic : base.italic,
  };
}

export function hydrateSettings(raw: unknown): AppSettings {
  const out = defaultSettings();
  const rec = asRecord(raw);
  if (!rec) {
    return out;
  }
  const general = pickRecord(rec, "general", "General");
  if (general) {
    const theme = general.theme ?? general.Theme;
    const language = general.language ?? general.Language;
    const startup = general.startup ?? general.Startup;
    const autoReadonly = general.autoReadonly ?? general.AutoReadonly;
    const resourceDirectory = general.resourceDirectory ?? general.ResourceDirectory;
    const showResourceDirectory = general.showResourceDirectory ?? general.ShowResourceDirectory;
    const maxDocTabs = general.maxDocTabs ?? general.MaxDocTabs;
    const docTabsLayout = general.docTabsLayout ?? general.DocTabsLayout;
    const autoCheckUpdates = general.autoCheckUpdates ?? general.AutoCheckUpdates;
    if (typeof theme === "string") {
      out.general.theme = normalizeTheme(theme);
    }
    if (typeof language === "string") {
      out.general.language = normalizeLanguage(language);
    }
    if (typeof startup === "string") {
      out.general.startup = normalizeStartup(startup);
    }
    if (typeof autoReadonly === "boolean") {
      out.general.autoReadonly = autoReadonly;
    }
    if (typeof resourceDirectory === "string" && resourceDirectory.trim()) {
      out.general.resourceDirectory = resourceDirectory;
    }
    if (typeof showResourceDirectory === "boolean") {
      out.general.showResourceDirectory = showResourceDirectory;
    }
    if (typeof maxDocTabs === "number") {
      out.general.maxDocTabs = normalizeMaxDocTabs(maxDocTabs);
    }
    if (typeof docTabsLayout === "string") {
      out.general.docTabsLayout = normalizeDocTabsLayout(docTabsLayout);
    }
    if (typeof autoCheckUpdates === "boolean") {
      out.general.autoCheckUpdates = autoCheckUpdates;
    }
  }
  const shortcuts = pickRecord(rec, "shortcuts", "Shortcuts");
  if (shortcuts) {
    out.shortcuts = {
      searchCurrentFile: readShortcut(shortcuts.searchCurrentFile ?? shortcuts.SearchCurrentFile, out.shortcuts.searchCurrentFile),
      searchWorkspace: readShortcut(shortcuts.searchWorkspace ?? shortcuts.SearchWorkspace, out.shortcuts.searchWorkspace),
      closeTab: readShortcut(shortcuts.closeTab ?? shortcuts.CloseTab, out.shortcuts.closeTab),
    };
  }
  const markdown = pickRecord(rec, "markdown", "Markdown");
  if (!markdown) {
    return out;
  }
  const width = markdown.editorWidth ?? markdown.EditorWidth;
  if (typeof width === "string" && width) {
    out.markdown.editorWidth = width;
  }
  const showHeadingPanel = markdown.showHeadingPanel ?? markdown.ShowHeadingPanel;
  if (typeof showHeadingPanel === "boolean") {
    out.markdown.showHeadingPanel = showHeadingPanel;
  }
  const styles = pickRecord(markdown, "styles", "Styles");
  if (!styles) {
    return out;
  }
  for (const key of STYLE_KEYS) {
    out.markdown.styles[key] = applyStyleOverlay(out.markdown.styles[key], styles[key] ?? styles[key[0]!.toUpperCase() + key.slice(1)]);
  }
  return out;
}

export function normalizeTheme(value: string): ThemeMode {
  return value === "light" || value === "dark" || value === "system" ? value : "system";
}

export function normalizeLanguage(value: string): Language {
  return value === "en" ? "en" : "zh-CN";
}

export function normalizeStartup(value: string): StartupMode {
  return value === "launch" ? "launch" : "last";
}

export function normalizeMaxDocTabs(value: number): number {
  return Math.min(100, Math.max(1, Math.round(Number.isFinite(value) ? value : 10)));
}

export function normalizeDocTabsLayout(value: string): DocTabsLayout {
  return value === "multi" ? "multi" : "single";
}

function readShortcut(value: unknown, fallback: string): string {
  if (typeof value !== "string") {
    return fallback;
  }
  return value.trim();
}

export function resolveTheme(theme: string, systemDark = prefersDark()): "light" | "dark" {
  const mode = normalizeTheme(theme);
  if (mode === "system") {
    return systemDark ? "dark" : "light";
  }
  return mode;
}

export function prefersDark() {
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
}

export function parseEditorWidth(value: string): { amount: string; unit: "px" | "%" } {
  const match = /^(\d+(?:\.\d+)?)\s*(px|%)?$/i.exec(value.trim());
  if (!match) {
    return { amount: "750", unit: "px" };
  }
  return { amount: match[1]!, unit: match[2]?.toLowerCase() === "%" ? "%" : "px" };
}

export function editorWidthCSS(value: string): string {
  const { amount, unit } = parseEditorWidth(value);
  const n = Number(amount);
  if (!Number.isFinite(n)) {
    return "750px";
  }
  if (unit === "%") {
    return `${Math.min(100, Math.max(10, n))}%`;
  }
  return `${Math.min(10000, Math.max(200, n))}px`;
}

export function formatEditorWidth(amount: string, unit: "px" | "%"): string {
  return `${amount}${unit}`;
}

export function sameMarkdownStyle(a: MarkdownStyle, b: MarkdownStyle) {
  return (
    a.color.light === b.color.light &&
    a.color.dark === b.color.dark &&
    a.background.light === b.background.light &&
    a.background.dark === b.background.dark &&
    a.fontSize === b.fontSize &&
    a.fontFamily === b.fontFamily &&
    a.bold === b.bold &&
    a.italic === b.italic
  );
}

function pickColor(color: ThemeColor, theme: "light" | "dark") {
  return theme === "dark" ? color.dark : color.light;
}

function decoration(key: StyleKey) {
  if (key === "strikethrough" || key === "taskChecked") {
    return "line-through";
  }
  return "none";
}

const NESTABLE_MARKS = new Set<StyleKey>(["strong", "emphasis", "strikethrough", "link", "inlineCode"]);

export function markdownStyleVars(styles: MarkdownStyles, theme: "light" | "dark"): Record<string, string> {
  const vars: Record<string, string> = {};
  for (const key of STYLE_KEYS) {
    const item = styles[key];
    vars[`--md-${key}-color`] = pickColor(item.color, theme);
    vars[`--md-${key}-bg`] = pickColor(item.background, theme);
    vars[`--md-${key}-size`] = item.fontSize;
    vars[`--md-${key}-font`] = item.fontFamily;
    vars[`--md-${key}-weight`] = item.bold ? "700" : NESTABLE_MARKS.has(key) ? "inherit" : "400";
    vars[`--md-${key}-style`] = item.italic ? "italic" : NESTABLE_MARKS.has(key) ? "inherit" : "normal";
    vars[`--md-${key}-decoration`] = decoration(key);
  }
  return vars;
}

export const FONT_PRESETS = [
  { id: "system", value: FONT_SANS },
  { id: "serif", value: `Georgia, "Times New Roman", Times, serif` },
  { id: "mono", value: FONT_MONO },
  { id: "song", value: `"Songti SC", SimSun, "Songti TC", serif` },
  { id: "hei", value: `"Heiti SC", SimHei, "PingFang SC", sans-serif` },
  { id: "kai", value: `"Kaiti SC", KaiTi, "STKaiti", serif` },
];
