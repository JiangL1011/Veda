import { useEffect, useState, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { AppInfo, UpdateState } from "../../bindings/veda/models";
import { AppService } from "../lib/api";
import { fillTemplate } from "../lib/search";
import { useSettings } from "../lib/SettingsContext";
import {
  FONT_PRESETS,
  STYLE_KEYS,
  defaultSettings,
  editorWidthCSS,
  formatEditorWidth,
  normalizeMaxDocTabs,
  parseEditorWidth,
  sameMarkdownStyle,
  type AppSettings,
  type MarkdownStyle,
  type ShortcutSettings,
  type StyleKey,
  type StartupMode,
  type ThemeMode,
} from "../lib/settings";
import {
  chordFromEvent,
  formatShortcut,
  normalizeShortcut,
  setRecordingShortcut,
} from "../lib/shortcuts";
import { cn } from "../lib/utils";
import { BoldIcon, CheckCircleIcon, CloseIcon, GearIcon, ItalicIcon, RefreshIcon } from "./Icons";
import { MarkdownPreview } from "./MarkdownPreview";

type Page = "general" | "markdown" | "shortcuts" | "about";

const LOGO_URL = `${import.meta.env.BASE_URL}veda-logo.png`;
const UPDATE_POLL_INTERVAL = 400;

const PREVIEW_MD = `# Heading 1
## Heading 2
Paragraph with **bold**, *italic*, ~~strike~~, \`code\` and [link](https://example.com).

- Unordered
1. Ordered
- [x] Done
- [ ] Todo

> Quote

\`\`\`
const hello = "world";
\`\`\`

| A | B |
| --- | --- |
| 1 | 2 |

---
`;

function hexForPicker(value: string) {
  const v = value.trim();
  return /^#[0-9a-fA-F]{6}$/.test(v) ? v : "#888888";
}

function ColorField({
  label,
  value,
  onChange,
  allowTransparent,
  transparentLabel,
}: {
  label: string;
  value: string;
  onChange: (next: string) => void;
  allowTransparent?: boolean;
  transparentLabel: string;
}) {
  const transparent = value.trim().toLowerCase() === "transparent" || value.trim() === "";
  return (
    <label className="flex min-w-0 flex-1 flex-col gap-1 text-xs text-[var(--vd-fg-muted)]">
      <span>{label}</span>
      <div className="flex items-center gap-1.5">
        <input
          type="color"
          aria-label={label}
          value={hexForPicker(transparent ? "#00000000" : value)}
          className="h-8 w-8 shrink-0 cursor-pointer rounded border border-[var(--vd-border)] bg-transparent p-0"
          onChange={(e) => onChange(e.target.value)}
        />
        <input
          type="text"
          spellCheck={false}
          value={value}
          className="min-w-0 flex-1 rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-2 py-1.5 text-[13px] text-[var(--vd-fg)] outline-none focus:border-[var(--vd-fg-muted)]"
          onChange={(e) => onChange(e.target.value)}
        />
        {allowTransparent && (
          <button
            type="button"
            className={cn(
              "shrink-0 rounded-md border px-2 py-1.5 text-[11px]",
              transparent
                ? "border-[var(--vd-fg)] text-[var(--vd-fg)]"
                : "border-[var(--vd-border)] text-[var(--vd-fg-muted)]",
            )}
            onClick={() => onChange("transparent")}
          >
            {transparentLabel}
          </button>
        )}
      </div>
    </label>
  );
}

function ItemAction({
  label,
  onClick,
  disabled,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      className={cn(
        "h-7 shrink-0 rounded-md border border-[var(--vd-border)] px-2 text-[11px] leading-none text-[var(--vd-fg-muted)]",
        disabled ? "cursor-default opacity-40" : "hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]",
      )}
      onClick={onClick}
    >
      {label}
    </button>
  );
}

function ItemHeader({
  title,
  action,
  reserveLabel,
}: {
  title: ReactNode;
  action?: ReactNode;
  reserveLabel: string;
}) {
  return (
    <div className="mb-3 flex h-7 items-center justify-between gap-2">
      <div className="min-w-0 text-sm font-medium text-[var(--vd-fg)]">{title}</div>
      <div className="flex h-7 shrink-0 items-center justify-end">
        {action ?? (
          <span
            aria-hidden
            className="invisible inline-flex h-7 items-center rounded-md border px-2 text-[11px] leading-none"
          >
            {reserveLabel}
          </span>
        )}
      </div>
    </div>
  );
}

function ToggleButton({
  pressed,
  title,
  onClick,
  children,
}: {
  pressed: boolean;
  title: string;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      title={title}
      aria-pressed={pressed}
      className={cn(
        "flex h-8 w-8 items-center justify-center rounded-md border",
        pressed
          ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
          : "border-[var(--vd-border)] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
      )}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

function ShortcutField({
  label,
  hint,
  value,
  t,
  onChange,
  followGlobal,
  conflict,
}: {
  label: string;
  hint?: string;
  value: string;
  t: (key: string) => string;
  onChange: (next: string) => void;
  followGlobal?: { label: string; onClick: () => void; disabled: boolean };
  conflict?: boolean;
}) {
  const [listening, setListening] = useState(false);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;

  useEffect(() => {
    if (!listening) {
      return;
    }
    setRecordingShortcut(true);
    const onKey = (event: KeyboardEvent) => {
      event.preventDefault();
      event.stopPropagation();
      if (event.key === "Escape") {
        setListening(false);
        return;
      }
      if (event.key === "Backspace" || event.key === "Delete") {
        onChangeRef.current("");
        setListening(false);
        return;
      }
      const chord = chordFromEvent(event);
      if (!chord) {
        return;
      }
      onChangeRef.current(normalizeShortcut(chord));
      setListening(false);
    };
    window.addEventListener("keydown", onKey, true);
    return () => {
      setRecordingShortcut(false);
      window.removeEventListener("keydown", onKey, true);
    };
  }, [listening]);

  return (
    <section className="rounded-xl border border-[var(--vd-border)] bg-[var(--vd-bg)] p-3">
      <ItemHeader
        title={label}
        reserveLabel={t("settings.followGlobal")}
        action={
          followGlobal ? (
            <ItemAction label={followGlobal.label} onClick={followGlobal.onClick} disabled={followGlobal.disabled} />
          ) : undefined
        }
      />
      <button
        type="button"
        onClick={() => setListening(true)}
        className={cn(
          "flex h-9 w-full items-center rounded-md border px-3 text-left text-sm outline-none",
          listening
            ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
            : conflict
              ? "border-red-500/70 text-[var(--vd-fg)]"
              : "border-[var(--vd-border)] text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]",
        )}
      >
        {listening
          ? t("settings.shortcuts.press")
          : value
            ? formatShortcut(value)
            : t("settings.shortcuts.empty")}
      </button>
      {hint && <p className="mt-2 text-xs text-[var(--vd-fg-muted)]">{hint}</p>}
      {conflict && <p className="mt-2 text-xs text-red-600">{t("settings.shortcuts.conflict")}</p>}
    </section>
  );
}

const SHORTCUT_KEYS = ["searchCurrentFile", "searchWorkspace", "closeTab"] as const;
type ShortcutKey = (typeof SHORTCUT_KEYS)[number];

function conflictingShortcuts(shortcuts: ShortcutSettings): Set<ShortcutKey> {
  const owners = new Map<string, ShortcutKey>();
  const clashing = new Set<ShortcutKey>();
  for (const key of SHORTCUT_KEYS) {
    const chord = normalizeShortcut(shortcuts[key]);
    if (!chord) {
      continue;
    }
    const owner = owners.get(chord);
    if (owner) {
      clashing.add(owner);
      clashing.add(key);
      continue;
    }
    owners.set(chord, key);
  }
  return clashing;
}

function ShortcutsPane({
  settings,
  t,
  onChange,
  followGlobal,
  globalSettings,
}: {
  settings: AppSettings;
  t: (key: string) => string;
  onChange: (next: AppSettings) => void;
  followGlobal?: boolean;
  globalSettings?: AppSettings;
}) {
  const source = globalSettings ?? defaultSettings();
  const follow = !!followGlobal;
  const conflicts = conflictingShortcuts(settings.shortcuts);

  const setShortcut = (key: ShortcutKey, next: string) => {
    onChange({
      ...settings,
      shortcuts: { ...settings.shortcuts, [key]: next },
    });
  };

  return (
    <div className="space-y-4">
      <p className="text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.shortcuts.hint")}</p>
      {SHORTCUT_KEYS.map((key) => (
        <ShortcutField
          key={key}
          label={t(`settings.shortcuts.${key}`)}
          hint={key === "closeTab" ? t("settings.shortcuts.closeTabHint") : undefined}
          value={settings.shortcuts[key]}
          t={t}
          conflict={conflicts.has(key)}
          onChange={(next) => setShortcut(key, next)}
          followGlobal={
            follow
              ? {
                  label: t("settings.followGlobal"),
                  disabled: settings.shortcuts[key] === source.shortcuts[key],
                  onClick: () => setShortcut(key, source.shortcuts[key]),
                }
              : undefined
          }
        />
      ))}
    </div>
  );
}

function StyleCard({
  styleKey,
  value,
  t,
  onChange,
  followGlobal,
}: {
  styleKey: StyleKey;
  value: MarkdownStyle;
  t: (key: string) => string;
  onChange: (next: MarkdownStyle) => void;
  followGlobal?: { label: string; onClick: () => void; disabled: boolean };
}) {
  return (
    <section className="rounded-xl border border-[var(--vd-border)] bg-[var(--vd-bg)] p-3">
      <ItemHeader
        title={t(`style.${styleKey}`)}
        reserveLabel={t("settings.followGlobal")}
        action={
          followGlobal ? (
            <ItemAction label={followGlobal.label} disabled={followGlobal.disabled} onClick={followGlobal.onClick} />
          ) : undefined
        }
      />
      <div className="grid gap-2 sm:grid-cols-2">
        <ColorField
          label={t("settings.colorLight")}
          value={value.color.light}
          onChange={(light) => onChange({ ...value, color: { ...value.color, light } })}
          transparentLabel={t("settings.transparent")}
        />
        <ColorField
          label={t("settings.colorDark")}
          value={value.color.dark}
          onChange={(dark) => onChange({ ...value, color: { ...value.color, dark } })}
          transparentLabel={t("settings.transparent")}
        />
        <ColorField
          label={t("settings.bgLight")}
          value={value.background.light}
          onChange={(light) => onChange({ ...value, background: { ...value.background, light } })}
          allowTransparent
          transparentLabel={t("settings.transparent")}
        />
        <ColorField
          label={t("settings.bgDark")}
          value={value.background.dark}
          onChange={(dark) => onChange({ ...value, background: { ...value.background, dark } })}
          allowTransparent
          transparentLabel={t("settings.transparent")}
        />
      </div>
      <div className="mt-3 grid gap-2 sm:grid-cols-[140px_1fr_auto]">
        <label className="flex flex-col gap-1 text-xs text-[var(--vd-fg-muted)]">
          <span>{t("settings.fontSize")}</span>
          <input
            type="text"
            spellCheck={false}
            value={value.fontSize}
            className="rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-2 py-1.5 text-[13px] text-[var(--vd-fg)] outline-none focus:border-[var(--vd-fg-muted)]"
            onChange={(e) => onChange({ ...value, fontSize: e.target.value })}
          />
        </label>
        <label className="flex min-w-0 flex-col gap-1 text-xs text-[var(--vd-fg-muted)]">
          <span>{t("settings.fontFamily")}</span>
          <input
            type="text"
            list={`font-presets-${styleKey}`}
            spellCheck={false}
            value={value.fontFamily}
            className="rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-2 py-1.5 text-[13px] text-[var(--vd-fg)] outline-none focus:border-[var(--vd-fg-muted)]"
            onChange={(e) => onChange({ ...value, fontFamily: e.target.value })}
          />
          <datalist id={`font-presets-${styleKey}`}>
            {FONT_PRESETS.map((preset) => (
              <option key={preset.id} value={preset.value} label={t(`font.${preset.id}`)} />
            ))}
          </datalist>
        </label>
        <div className="flex items-end gap-1">
          <ToggleButton
            pressed={value.bold}
            title={t("settings.bold")}
            onClick={() => onChange({ ...value, bold: !value.bold })}
          >
            <BoldIcon className="h-4 w-4" />
          </ToggleButton>
          <ToggleButton
            pressed={value.italic}
            title={t("settings.italic")}
            onClick={() => onChange({ ...value, italic: !value.italic })}
          >
            <ItalicIcon className="h-4 w-4" />
          </ToggleButton>
        </div>
      </div>
    </section>
  );
}

function GeneralPane({
  settings,
  t,
  onChange,
  followGlobal,
  globalSettings,
  showStartup,
}: {
  settings: AppSettings;
  t: (key: string) => string;
  onChange: (next: AppSettings) => void;
  followGlobal?: boolean;
  globalSettings?: AppSettings;
  showStartup?: boolean;
}) {
  const source = globalSettings ?? defaultSettings();
  const theme = (settings.general.theme === "light" || settings.general.theme === "dark"
    ? settings.general.theme
    : "system") as ThemeMode;
  const language = settings.general.language === "en" ? "en" : "zh-CN";
  const startup = (settings.general.startup === "launch" ? "launch" : "last") as StartupMode;
  const follow = !!followGlobal;

  return (
    <div className="space-y-8">
      <fieldset>
        <ItemHeader
          title={t("settings.theme")}
          reserveLabel={t("settings.followGlobal")}
          action={
            follow ? (
              <ItemAction
                label={t("settings.followGlobal")}
                disabled={settings.general.theme === source.general.theme}
                onClick={() =>
                  onChange({ ...settings, general: { ...settings.general, theme: source.general.theme } })
                }
              />
            ) : undefined
          }
        />
        <div className="flex flex-wrap gap-2">
          {(["light", "dark", "system"] as const).map((value) => (
            <button
              key={value}
              type="button"
              className={cn(
                "rounded-lg border px-3 py-1.5 text-sm",
                theme === value
                  ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                  : "border-[var(--vd-border)] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
              )}
              onClick={() =>
                onChange({ ...settings, general: { ...settings.general, theme: value } })
              }
            >
              {t(`settings.theme.${value}`)}
            </button>
          ))}
        </div>
      </fieldset>
      <fieldset>
        <ItemHeader
          title={t("settings.language")}
          reserveLabel={t("settings.followGlobal")}
          action={
            follow ? (
              <ItemAction
                label={t("settings.followGlobal")}
                disabled={settings.general.language === source.general.language}
                onClick={() =>
                  onChange({
                    ...settings,
                    general: { ...settings.general, language: source.general.language },
                  })
                }
              />
            ) : undefined
          }
        />
        <div className="flex flex-wrap gap-2">
          {(["zh-CN", "en"] as const).map((value) => (
            <button
              key={value}
              type="button"
              className={cn(
                "rounded-lg border px-3 py-1.5 text-sm",
                language === value
                  ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                  : "border-[var(--vd-border)] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
              )}
              onClick={() =>
                onChange({ ...settings, general: { ...settings.general, language: value } })
              }
            >
              {t(value === "zh-CN" ? "settings.language.zh" : "settings.language.en")}
            </button>
          ))}
        </div>
      </fieldset>
      <fieldset>
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-[var(--vd-fg)]">{t("settings.maxDocTabs")}</h3>
            <p className="mt-1 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.maxDocTabsHint")}</p>
          </div>
          {follow && (
            <ItemAction
              label={t("settings.followGlobal")}
              disabled={settings.general.maxDocTabs === source.general.maxDocTabs}
              onClick={() =>
                onChange({
                  ...settings,
                  general: { ...settings.general, maxDocTabs: source.general.maxDocTabs },
                })
              }
            />
          )}
        </div>
        <input
          type="number"
          min={1}
          max={100}
          step={1}
          value={settings.general.maxDocTabs}
          className="mt-3 w-28 rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-3 py-2 text-sm text-[var(--vd-fg)] outline-none focus:border-[var(--vd-fg-muted)]"
          onChange={(event) =>
            onChange({
              ...settings,
              general: {
                ...settings.general,
                maxDocTabs: normalizeMaxDocTabs(Number(event.target.value)),
              },
            })
          }
        />
      </fieldset>
      <fieldset>
        <ItemHeader
          title={t("settings.docTabsLayout")}
          reserveLabel={t("settings.followGlobal")}
          action={
            follow ? (
              <ItemAction
                label={t("settings.followGlobal")}
                disabled={settings.general.docTabsLayout === source.general.docTabsLayout}
                onClick={() =>
                  onChange({
                    ...settings,
                    general: { ...settings.general, docTabsLayout: source.general.docTabsLayout },
                  })
                }
              />
            ) : undefined
          }
        />
        <div className="flex flex-wrap gap-2">
          {(["single", "multi"] as const).map((value) => (
            <button
              key={value}
              type="button"
              className={cn(
                "rounded-lg border px-3 py-1.5 text-sm",
                settings.general.docTabsLayout === value
                  ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                  : "border-[var(--vd-border)] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
              )}
              onClick={() =>
                onChange({
                  ...settings,
                  general: { ...settings.general, docTabsLayout: value },
                })
              }
            >
              {t(`settings.docTabsLayout.${value}`)}
            </button>
          ))}
        </div>
      </fieldset>
      <fieldset>
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-[var(--vd-fg)]">{t("settings.resourceDirectory")}</h3>
            <p className="mt-1 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.resourceDirectoryHint")}</p>
          </div>
          {follow && (
            <ItemAction
              label={t("settings.followGlobal")}
              disabled={settings.general.resourceDirectory === source.general.resourceDirectory}
              onClick={() =>
                onChange({
                  ...settings,
                  general: { ...settings.general, resourceDirectory: source.general.resourceDirectory },
                })
              }
            />
          )}
        </div>
        <input
          type="text"
          spellCheck={false}
          value={settings.general.resourceDirectory}
          placeholder="assets"
          className="mt-3 w-full rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-3 py-2 text-sm text-[var(--vd-fg)] outline-none focus:border-[var(--vd-fg-muted)]"
          onChange={(e) =>
            onChange({
              ...settings,
              general: { ...settings.general, resourceDirectory: e.target.value },
            })
          }
        />
      </fieldset>
      <fieldset>
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-[var(--vd-fg)]">{t("settings.showResourceDirectory")}</h3>
            <p className="mt-1 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.showResourceDirectoryHint")}</p>
          </div>
          {follow && (
            <ItemAction
              label={t("settings.followGlobal")}
              disabled={settings.general.showResourceDirectory === source.general.showResourceDirectory}
              onClick={() =>
                onChange({
                  ...settings,
                  general: {
                    ...settings.general,
                    showResourceDirectory: source.general.showResourceDirectory,
                  },
                })
              }
            />
          )}
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          {([true, false] as const).map((value) => (
            <button
              key={value ? "show" : "hide"}
              type="button"
              className={cn(
                "rounded-lg border px-3 py-1.5 text-sm",
                settings.general.showResourceDirectory === value
                  ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                  : "border-[var(--vd-border)] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
              )}
              onClick={() =>
                onChange({
                  ...settings,
                  general: { ...settings.general, showResourceDirectory: value },
                })
              }
            >
              {t(value ? "settings.resourceDirectory.show" : "settings.resourceDirectory.hide")}
            </button>
          ))}
        </div>
      </fieldset>
      <fieldset>
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-[var(--vd-fg)]">{t("settings.autoReadonly")}</h3>
            <p className="mt-1 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.autoReadonlyHint")}</p>
          </div>
          <div className="flex h-7 shrink-0 items-center justify-end">
            {follow ? (
              <ItemAction
                label={t("settings.followGlobal")}
                disabled={settings.general.autoReadonly === source.general.autoReadonly}
                onClick={() =>
                  onChange({
                    ...settings,
                    general: { ...settings.general, autoReadonly: source.general.autoReadonly },
                  })
                }
              />
            ) : (
              <span
                aria-hidden
                className="invisible inline-flex h-7 items-center rounded-md border px-2 text-[11px] leading-none"
              >
                {t("settings.followGlobal")}
              </span>
            )}
          </div>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          {([true, false] as const).map((value) => (
            <button
              key={value ? "on" : "off"}
              type="button"
              className={cn(
                "rounded-lg border px-3 py-1.5 text-sm",
                settings.general.autoReadonly === value
                  ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                  : "border-[var(--vd-border)] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
              )}
              onClick={() =>
                onChange({ ...settings, general: { ...settings.general, autoReadonly: value } })
              }
            >
              {t(value ? "settings.autoReadonly.on" : "settings.autoReadonly.off")}
            </button>
          ))}
        </div>
      </fieldset>
      {showStartup && (
        <fieldset>
          <ItemHeader title={t("settings.startup")} reserveLabel={t("settings.followGlobal")} />
          <div className="flex flex-wrap gap-2">
            {(["last", "launch"] as const).map((value) => (
              <button
                key={value}
                type="button"
                className={cn(
                  "rounded-lg border px-3 py-1.5 text-sm",
                  startup === value
                    ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                    : "border-[var(--vd-border)] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
                )}
                onClick={() =>
                  onChange({ ...settings, general: { ...settings.general, startup: value } })
                }
              >
                {t(`settings.startup.${value}`)}
              </button>
            ))}
          </div>
        </fieldset>
      )}
    </div>
  );
}

function MarkdownPane({
  settings,
  t,
  onChange,
  followGlobal,
  globalSettings,
}: {
  settings: AppSettings;
  t: (key: string) => string;
  onChange: (next: AppSettings) => void;
  followGlobal?: boolean;
  globalSettings?: AppSettings;
}) {
  const source = globalSettings ?? defaultSettings();
  const parsed = parseEditorWidth(settings.markdown.editorWidth);
  const follow = !!followGlobal;

  return (
    <div className="space-y-6">
      <section>
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-[var(--vd-fg)]">{t("settings.headingPanel")}</h3>
            <p className="mt-1 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.headingPanelHint")}</p>
          </div>
          <div className="flex h-7 shrink-0 items-center justify-end">
            {follow ? (
              <ItemAction
                label={t("settings.followGlobal")}
                disabled={settings.markdown.showHeadingPanel === source.markdown.showHeadingPanel}
                onClick={() =>
                  onChange({
                    ...settings,
                    markdown: {
                      ...settings.markdown,
                      showHeadingPanel: source.markdown.showHeadingPanel,
                    },
                  })
                }
              />
            ) : (
              <span
                aria-hidden
                className="invisible inline-flex h-7 items-center rounded-md border px-2 text-[11px] leading-none"
              >
                {t("settings.followGlobal")}
              </span>
            )}
          </div>
        </div>
        <div className="mt-3 flex flex-wrap gap-2">
          {([true, false] as const).map((value) => (
            <button
              key={value ? "show" : "hide"}
              type="button"
              className={cn(
                "rounded-lg border px-3 py-1.5 text-sm",
                settings.markdown.showHeadingPanel === value
                  ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                  : "border-[var(--vd-border)] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
              )}
              onClick={() =>
                onChange({
                  ...settings,
                  markdown: { ...settings.markdown, showHeadingPanel: value },
                })
              }
            >
              {t(value ? "settings.headingPanel.show" : "settings.headingPanel.hide")}
            </button>
          ))}
        </div>
      </section>

      <section>
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-[var(--vd-fg)]">{t("settings.editorWidth")}</h3>
            <p className="mt-1 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.editorWidthHint")}</p>
          </div>
          <div className="flex h-7 shrink-0 items-center justify-end">
            {follow ? (
              <ItemAction
                label={t("settings.followGlobal")}
                disabled={settings.markdown.editorWidth === source.markdown.editorWidth}
                onClick={() =>
                  onChange({
                    ...settings,
                    markdown: { ...settings.markdown, editorWidth: source.markdown.editorWidth },
                  })
                }
              />
            ) : (
              <span
                aria-hidden
                className="invisible inline-flex h-7 items-center rounded-md border px-2 text-[11px] leading-none"
              >
                {t("settings.followGlobal")}
              </span>
            )}
          </div>
        </div>
        <div className="mt-3 flex items-center gap-2">
          <input
            type="text"
            inputMode="decimal"
            value={parsed.amount}
            className="w-28 rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-2 py-1.5 text-sm text-[var(--vd-fg)] outline-none focus:border-[var(--vd-fg-muted)]"
            onChange={(e) =>
              onChange({
                ...settings,
                markdown: {
                  ...settings.markdown,
                  editorWidth: formatEditorWidth(e.target.value, parsed.unit),
                },
              })
            }
          />
          <div className="flex overflow-hidden rounded-md border border-[var(--vd-border)]">
            {(["px", "%"] as const).map((unit) => (
              <button
                key={unit}
                type="button"
                className={cn(
                  "px-3 py-1.5 text-sm",
                  parsed.unit === unit
                    ? "bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                    : "text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
                )}
                onClick={() =>
                  onChange({
                    ...settings,
                    markdown: {
                      ...settings.markdown,
                      editorWidth: formatEditorWidth(parsed.amount, unit),
                    },
                  })
                }
              >
                {unit === "px" ? t("settings.unit.px") : t("settings.unit.percent")}
              </button>
            ))}
          </div>
        </div>
      </section>

      <section>
        <h3 className="text-sm font-medium text-[var(--vd-fg)]">{t("settings.customStyles")}</h3>
        <p className="mt-1 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.customStylesHint")}</p>
        <div className="mt-3 overflow-hidden rounded-xl border border-[var(--vd-border)] bg-[var(--vd-bg-subtle)]">
          <div className="border-b border-[var(--vd-border)] px-3 py-2 text-[11px] font-medium tracking-wide text-[var(--vd-fg-subtle)] uppercase">
            {t("settings.preview")}
          </div>
          <div className="px-4 py-4">
            <MarkdownPreview
              markdown={PREVIEW_MD}
              className="mx-auto"
              style={{ width: editorWidthCSS(settings.markdown.editorWidth), maxWidth: "100%" }}
            />
          </div>
        </div>
        <div className="mt-4 space-y-3">
          {STYLE_KEYS.map((key) => (
            <StyleCard
              key={key}
              styleKey={key}
              value={settings.markdown.styles[key]}
              t={t}
              followGlobal={
                follow
                  ? {
                      label: t("settings.followGlobal"),
                      disabled: sameMarkdownStyle(settings.markdown.styles[key], source.markdown.styles[key]),
                      onClick: () =>
                        onChange({
                          ...settings,
                          markdown: {
                            ...settings.markdown,
                            styles: {
                              ...settings.markdown.styles,
                              [key]: structuredClone(source.markdown.styles[key]),
                            },
                          },
                        }),
                    }
                  : undefined
              }
              onChange={(next) =>
                onChange({
                  ...settings,
                  markdown: {
                    ...settings.markdown,
                    styles: { ...settings.markdown.styles, [key]: next },
                  },
                })
              }
            />
          ))}
        </div>
      </section>
    </div>
  );
}

function ToggleSwitch({
  checked,
  label,
  onChange,
}: {
  checked: boolean;
  label: string;
  onChange: (next: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className={cn(
        "relative h-6 w-11 shrink-0 rounded-full border transition-colors",
        checked ? "border-[var(--vd-fg)] bg-[var(--vd-fg)]" : "border-[var(--vd-border)] bg-[var(--vd-bg-subtle)]",
      )}
      onClick={() => onChange(!checked)}
    >
      <span
        aria-hidden
        className={cn(
          "absolute top-[2px] h-[18px] w-[18px] rounded-full bg-[var(--vd-bg)] shadow-sm transition-[left]",
          checked ? "left-[22px]" : "left-[2px]",
        )}
      />
    </button>
  );
}

function platformName(platform: string) {
  switch (platform) {
    case "darwin":
      return "macOS";
    case "windows":
      return "Windows";
    case "linux":
      return "Linux";
    default:
      return platform || "—";
  }
}

function errorText(err: unknown) {
  if (err instanceof Error) {
    return err.message;
  }
  return typeof err === "string" ? err : String(err ?? "");
}

function AboutPane({ t }: { t: (key: string) => string }) {
  const { current, updateCurrent } = useSettings();
  const [info, setInfo] = useState<AppInfo | null>(null);
  const [state, setState] = useState<UpdateState | null>(null);
  const [actionError, setActionError] = useState("");
  const [installing, setInstalling] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void AppService.GetAppInfo()
      .then((value) => {
        if (!cancelled && value) {
          setInfo(value);
        }
      })
      .catch(() => undefined);
    void AppService.GetUpdateState()
      .then((value) => {
        if (!cancelled && value) {
          setState(value);
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  const status = state?.status ?? "idle";
  const busy = status === "checking" || status === "downloading";

  // 检查和下载都在后台进行，这里按固定间隔轮询状态。
  useEffect(() => {
    if (!busy) {
      return;
    }
    let cancelled = false;
    const timer = window.setInterval(() => {
      void AppService.GetUpdateState()
        .then((value) => {
          if (!cancelled && value) {
            setState(value);
          }
        })
        .catch(() => undefined);
    }, UPDATE_POLL_INTERVAL);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [busy]);

  const startCheck = () => {
    setActionError("");
    void AppService.CheckForUpdate()
      .then((value) => {
        if (value) {
          setState(value);
        }
      })
      .catch((err) => setActionError(errorText(err)));
  };

  const install = () => {
    setActionError("");
    setInstalling(true);
    void AppService.InstallUpdate().catch((err) => {
      setInstalling(false);
      setActionError(errorText(err));
    });
  };

  const openReleases = () => {
    void AppService.OpenURL(info?.releasesUrl || "https://github.com/JiangL1011/Veda/releases").catch(
      () => undefined,
    );
  };

  const percent = Math.round(Math.min(1, Math.max(0, state?.progress ?? 0)) * 100);
  const version = info?.version || state?.currentVersion || "";
  const ready = status === "ready";
  const autoCheck = current.general.autoCheckUpdates;

  let statusText = "";
  let statusTone: "muted" | "done" | "error" = "muted";
  if (actionError) {
    statusText = fillTemplate(t("settings.about.failed"), { error: actionError });
    statusTone = "error";
  } else if (status === "checking") {
    statusText = t("settings.about.checking");
  } else if (status === "downloading") {
    statusText = fillTemplate(t("settings.about.downloading"), { percent });
  } else if (status === "ready") {
    statusText = fillTemplate(t("settings.about.ready"), { version: state?.latestVersion ?? "" });
    statusTone = "done";
  } else if (status === "up-to-date") {
    statusText = t("settings.about.upToDate");
    statusTone = "done";
  } else if (status === "unsupported") {
    statusText = t("settings.about.unsupported");
  } else if (status === "error") {
    statusText = fillTemplate(t("settings.about.failed"), { error: state?.error ?? "" });
    statusTone = "error";
  }

  return (
    <div className="space-y-8">
      <section className="flex items-center gap-4 rounded-xl border border-[var(--vd-border)] bg-[var(--vd-bg-subtle)] p-4">
        <img src={LOGO_URL} alt="Veda" className="h-16 w-16 shrink-0 rounded-2xl" />
        <div className="min-w-0">
          <div className="text-lg font-semibold tracking-wide text-[var(--vd-fg)]">Veda</div>
          <p className="mt-0.5 text-xs text-[var(--vd-fg-muted)]">{t("settings.about.brandTagline")}</p>
          <dl className="mt-3 space-y-1 text-xs text-[var(--vd-fg-muted)]">
            <div className="flex items-baseline gap-2">
              <dt>{t("settings.about.version")}</dt>
              <dd className="font-medium text-[var(--vd-fg)]">{version || "—"}</dd>
            </div>
            <div className="flex items-baseline gap-2">
              <dt>{t("settings.about.platform")}</dt>
              <dd className="font-medium text-[var(--vd-fg)]">
                {info ? `${platformName(info.platform)} · ${info.arch}` : "—"}
              </dd>
            </div>
          </dl>
        </div>
      </section>

      <section>
        <div className="flex flex-wrap items-center gap-2">
          {ready ? (
            <button
              type="button"
              disabled={installing}
              className={cn(
                "inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-sm",
                installing
                  ? "cursor-default border-[var(--vd-border)] text-[var(--vd-fg-muted)] opacity-60"
                  : "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]",
              )}
              onClick={install}
            >
              <CheckCircleIcon className="h-4 w-4" />
              {installing ? t("settings.about.restarting") : t("settings.about.restart")}
            </button>
          ) : (
            <button
              type="button"
              disabled={busy}
              className={cn(
                "inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-sm",
                busy
                  ? "cursor-default border-[var(--vd-border)] text-[var(--vd-fg-muted)] opacity-60"
                  : "border-[var(--vd-fg)] text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]",
              )}
              onClick={startCheck}
            >
              <RefreshIcon className={cn("h-4 w-4", status === "checking" && "animate-spin")} />
              {status === "downloading"
                ? fillTemplate(t("settings.about.downloading"), { percent })
                : status === "checking"
                  ? t("settings.about.checking")
                  : t("settings.about.check")}
            </button>
          )}
          <ItemAction label={t("settings.about.releases")} onClick={openReleases} />
        </div>
        {statusText && (
          <p
            className={cn(
              "mt-3 text-xs leading-5",
              statusTone === "error"
                ? "text-red-600"
                : statusTone === "done"
                  ? "text-[var(--vd-fg)]"
                  : "text-[var(--vd-fg-muted)]",
            )}
          >
            {statusText}
          </p>
        )}
        <p className="mt-2 text-xs leading-5 text-[var(--vd-fg-subtle)]">
          {fillTemplate(t("settings.about.lastChecked"), {
            time: state?.checkedAt ? new Date(state.checkedAt).toLocaleString() : t("settings.about.never"),
          })}
        </p>
      </section>

      <section>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-[var(--vd-fg)]">{t("settings.about.autoCheck")}</h3>
            <p className="mt-1 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("settings.about.autoCheckHint")}</p>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs text-[var(--vd-fg-muted)]">
              {t(autoCheck ? "settings.about.on" : "settings.about.off")}
            </span>
            <ToggleSwitch
              checked={autoCheck}
              label={t("settings.about.autoCheck")}
              onChange={(next) =>
                updateCurrent({ ...current, general: { ...current.general, autoCheckUpdates: next } })
              }
            />
          </div>
        </div>
      </section>
    </div>
  );
}

export function SettingsModal() {
  const { current, globalSettings, updateCurrent, scope, setScope, canUseWorkspace, t, closeModal, modalOpen } =
    useSettings();
  const [page, setPage] = useState<Page>("general");
  const followGlobal = scope === "workspace" && canUseWorkspace;
  // 「关于」只在全局设置里出现。
  const showAbout = scope === "global";

  useEffect(() => {
    if (!showAbout && page === "about") {
      setPage("general");
    }
  }, [showAbout, page]);

  const resetCategory = () => {
    const defaults = defaultSettings();
    if (page === "general") {
      updateCurrent({ ...current, general: defaults.general });
      return;
    }
    if (page === "shortcuts") {
      updateCurrent({ ...current, shortcuts: defaults.shortcuts });
      return;
    }
    updateCurrent({ ...current, markdown: defaults.markdown });
  };

  if (!modalOpen) {
    return null;
  }

  return createPortal(
    <div className="titlebar-no-drag fixed inset-0 z-[80] flex items-center justify-center p-4 sm:p-8">
      <div className="absolute inset-0 bg-[var(--vd-overlay)] backdrop-blur-md" />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="settings-title"
        className="relative flex h-[min(760px,88vh)] w-[min(960px,94vw)] overflow-hidden rounded-2xl border border-[var(--vd-border)] bg-[var(--vd-bg)] shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <aside className="flex w-52 shrink-0 flex-col border-r border-[var(--vd-border)] bg-[var(--vd-bg-muted)]">
          <div className="p-3">
            <div className="flex rounded-lg bg-[var(--vd-bg-subtle)] p-0.5">
              <button
                type="button"
                className={cn(
                  "flex-1 rounded-md px-2 py-1.5 text-xs font-medium",
                  scope === "global"
                    ? "bg-[var(--vd-bg)] text-[var(--vd-fg)] shadow-sm"
                    : "text-[var(--vd-fg-muted)]",
                )}
                onClick={() => setScope("global")}
              >
                {t("settings.scope.global")}
              </button>
              <button
                type="button"
                disabled={!canUseWorkspace}
                title={canUseWorkspace ? undefined : t("settings.scope.fileOnly")}
                className={cn(
                  "flex-1 rounded-md px-2 py-1.5 text-xs font-medium",
                  !canUseWorkspace && "cursor-not-allowed opacity-40",
                  scope === "workspace" && canUseWorkspace
                    ? "bg-[var(--vd-bg)] text-[var(--vd-fg)] shadow-sm"
                    : "text-[var(--vd-fg-muted)]",
                )}
                onClick={() => setScope("workspace")}
              >
                {t("settings.scope.workspace")}
              </button>
            </div>
          </div>
          <nav className="flex flex-1 flex-col gap-0.5 px-2">
            {(
              [
                ["general", "settings.nav.general"],
                ["markdown", "settings.nav.markdown"],
                ["shortcuts", "settings.nav.shortcuts"],
                ["about", "settings.nav.about"],
              ] as const
            )
              .filter(([id]) => id !== "about" || showAbout)
              .map(([id, key]) => (
                <button
                  key={id}
                  type="button"
                  className={cn(
                    "rounded-lg px-3 py-2 text-left text-sm",
                    page === id
                      ? "bg-[var(--vd-selected)] font-medium text-[var(--vd-fg)]"
                      : "text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
                  )}
                  onClick={() => setPage(id)}
                >
                  {t(key)}
                </button>
              ))}
          </nav>
        </aside>
        <div className="flex min-w-0 flex-1 flex-col">
          <header className="flex h-12 shrink-0 items-center justify-between border-b border-[var(--vd-border)] px-5">
            <h2 id="settings-title" className="text-sm font-semibold text-[var(--vd-fg)]">
              {t("settings.title")}
            </h2>
            <div className="flex items-center gap-2">
              {scope === "global" && page !== "about" && (
                <ItemAction label={t("settings.resetDefaults")} onClick={resetCategory} />
              )}
              <button
                type="button"
                aria-label={t("settings.close")}
                className="flex h-8 w-8 items-center justify-center rounded-md text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
                onClick={closeModal}
              >
                <CloseIcon className="h-4 w-4" />
              </button>
            </div>
          </header>
          <div className="min-h-0 flex-1 overflow-auto px-5 py-5">
            {page === "about" ? (
              <AboutPane t={t} />
            ) : page === "general" ? (
              <GeneralPane
                settings={current}
                t={t}
                onChange={updateCurrent}
                followGlobal={followGlobal}
                globalSettings={globalSettings}
                showStartup={scope === "global"}
              />
            ) : page === "shortcuts" ? (
              <ShortcutsPane
                settings={current}
                t={t}
                onChange={updateCurrent}
                followGlobal={followGlobal}
                globalSettings={globalSettings}
              />
            ) : (
              <MarkdownPane
                settings={current}
                t={t}
                onChange={updateCurrent}
                followGlobal={followGlobal}
                globalSettings={globalSettings}
              />
            )}
          </div>
        </div>
      </div>
    </div>,
    document.body,
  );
}

export function SettingsButton({ className }: { className?: string }) {
  const { t, openModal } = useSettings();
  return (
    <button
      type="button"
      onClick={openModal}
      className={cn(
        "flex h-8 w-8 items-center justify-center rounded-md text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]",
        className,
      )}
      title={t("sidebar.settings")}
    >
      <GearIcon className="h-[18px] w-[18px]" />
    </button>
  );
}
