import { hostPlatform } from "./utils";

export const DEFAULT_SEARCH_CURRENT_FILE = "Mod+F";
export const DEFAULT_SEARCH_WORKSPACE = "Mod+Shift+F";

/* macOS 上是 Cmd+W，其余平台上 Ctrl+W 会被系统和 WebView 抢走，所以改用 Alt+W。
   这里必须和 Go 侧的 defaultCloseTab 保持一致。 */
export function defaultCloseTab(): string {
  return isMac() ? "Mod+W" : "Alt+W";
}

export type ShortcutChord = {
  mod: boolean;
  ctrl: boolean;
  meta: boolean;
  alt: boolean;
  shift: boolean;
  key: string;
};

const MODIFIER_ONLY = new Set(["Control", "Shift", "Alt", "Meta", "OS"]);

function isMac() {
  return hostPlatform() === "mac";
}

export function normalizeShortcut(raw: string): string {
  const chord = parseShortcut(raw);
  if (!chord) {
    return "";
  }
  return serializeChord(chord);
}

export function parseShortcut(raw: string): ShortcutChord | null {
  const parts = raw
    .split("+")
    .map((part) => part.trim())
    .filter(Boolean);
  if (parts.length === 0) {
    return null;
  }
  const chord: ShortcutChord = { mod: false, ctrl: false, meta: false, alt: false, shift: false, key: "" };
  for (const part of parts) {
    const token = part.toLowerCase();
    if (token === "mod" || token === "cmdorctrl" || token === "cmdorcontrol") {
      chord.mod = true;
      continue;
    }
    if (token === "ctrl" || token === "control") {
      chord.ctrl = true;
      continue;
    }
    if (token === "cmd" || token === "meta" || token === "command" || token === "super" || token === "win") {
      chord.meta = true;
      continue;
    }
    if (token === "alt" || token === "option" || token === "opt") {
      chord.alt = true;
      continue;
    }
    if (token === "shift") {
      chord.shift = true;
      continue;
    }
    chord.key = normalizeKeyName(part);
  }
  if (!chord.key) {
    return null;
  }
  return chord;
}

export function serializeChord(chord: ShortcutChord): string {
  const parts: string[] = [];
  if (chord.mod) {
    parts.push("Mod");
  }
  if (chord.ctrl) {
    parts.push("Ctrl");
  }
  if (chord.meta) {
    parts.push("Meta");
  }
  if (chord.alt) {
    parts.push("Alt");
  }
  if (chord.shift) {
    parts.push("Shift");
  }
  parts.push(chord.key);
  return parts.join("+");
}

export function normalizeKeyName(key: string): string {
  const trimmed = key.trim();
  if (!trimmed) {
    return "";
  }
  if (/^Key[A-Z]$/i.test(trimmed)) {
    return trimmed.slice(-1).toUpperCase();
  }
  if (/^Digit[0-9]$/i.test(trimmed)) {
    return trimmed.slice(-1);
  }
  if (trimmed.length === 1) {
    return trimmed.toUpperCase();
  }
  const named: Record<string, string> = {
    esc: "Escape",
    escape: "Escape",
    enter: "Enter",
    return: "Enter",
    space: "Space",
    " ": "Space",
    tab: "Tab",
    backspace: "Backspace",
    delete: "Delete",
    del: "Delete",
    arrowup: "ArrowUp",
    arrowdown: "ArrowDown",
    arrowleft: "ArrowLeft",
    arrowright: "ArrowRight",
    up: "ArrowUp",
    down: "ArrowDown",
    left: "ArrowLeft",
    right: "ArrowRight",
  };
  return named[trimmed.toLowerCase()] ?? trimmed[0]!.toUpperCase() + trimmed.slice(1);
}

export function chordFromEvent(event: KeyboardEvent): string | null {
  if (MODIFIER_ONLY.has(event.key)) {
    return null;
  }
  const mac = isMac();
  const chord: ShortcutChord = {
    mod: mac ? event.metaKey : event.ctrlKey,
    ctrl: mac ? event.ctrlKey : false,
    meta: mac ? false : event.metaKey,
    alt: event.altKey,
    shift: event.shiftKey,
    key: keyFromEvent(event),
  };
  if (!chord.key) {
    return null;
  }
  return serializeChord(chord);
}

function keyFromEvent(event: KeyboardEvent): string {
  if (/^Key[A-Z]$/.test(event.code)) {
    return event.code.slice(-1);
  }
  if (/^Digit[0-9]$/.test(event.code)) {
    return event.code.slice(-1);
  }
  if (/^F\d{1,2}$/.test(event.key)) {
    return event.key;
  }
  return normalizeKeyName(event.key);
}

export function matchesShortcut(event: KeyboardEvent, raw: string): boolean {
  const expected = parseShortcut(raw);
  if (!expected) {
    return false;
  }
  const actual = parseShortcut(chordFromEvent(event) ?? "");
  if (!actual) {
    return false;
  }
  return (
    actual.mod === expected.mod &&
    actual.ctrl === expected.ctrl &&
    actual.meta === expected.meta &&
    actual.alt === expected.alt &&
    actual.shift === expected.shift &&
    actual.key.toLowerCase() === expected.key.toLowerCase()
  );
}

export function formatShortcut(raw: string, platform = hostPlatform()): string {
  const chord = parseShortcut(raw);
  if (!chord) {
    return "";
  }
  const mac = platform === "mac";
  const parts: string[] = [];
  if (mac) {
    if (chord.ctrl) {
      parts.push("⌃");
    }
    if (chord.alt) {
      parts.push("⌥");
    }
    if (chord.shift) {
      parts.push("⇧");
    }
    if (chord.mod || chord.meta) {
      parts.push("⌘");
    }
    parts.push(displayKey(chord.key, true));
    return parts.join("");
  }
  if (chord.mod || chord.ctrl) {
    parts.push("Ctrl");
  }
  if (chord.meta) {
    parts.push("Win");
  }
  if (chord.alt) {
    parts.push("Alt");
  }
  if (chord.shift) {
    parts.push("Shift");
  }
  parts.push(displayKey(chord.key, false));
  return parts.join("+");
}

function displayKey(key: string, mac: boolean): string {
  const map: Record<string, string> = {
    Escape: mac ? "Esc" : "Esc",
    ArrowUp: "↑",
    ArrowDown: "↓",
    ArrowLeft: "←",
    ArrowRight: "→",
    Space: mac ? "Space" : "Space",
    Enter: mac ? "⏎" : "Enter",
    Backspace: mac ? "⌫" : "Backspace",
    Delete: mac ? "⌦" : "Delete",
    Tab: mac ? "⇥" : "Tab",
  };
  return map[key] ?? key.toUpperCase();
}

export function isRecordingShortcut(): boolean {
  return document.documentElement.dataset.recordingShortcut === "1";
}

export function setRecordingShortcut(active: boolean) {
  if (active) {
    document.documentElement.dataset.recordingShortcut = "1";
    return;
  }
  delete document.documentElement.dataset.recordingShortcut;
}

export function isFindFieldTarget(target: EventTarget | null) {
  return target instanceof HTMLElement && target.closest("[data-vd-find]");
}
