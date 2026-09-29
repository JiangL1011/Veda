import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { FileMeta, SearchMatch, TextFile } from "../../bindings/veda/models";
import { AppService } from "../lib/api";
import { EMPTY_FIND_MODES, searchOptions, type FindModes, type RevealTarget } from "../lib/search";
import { useSettings } from "../lib/SettingsContext";
import { cn } from "../lib/utils";
import { FindBar } from "./FindBar";
import { HeadingPanel } from "./HeadingPanel";
import { ChevronIcon, LockIcon } from "./Icons";
import {
  MarkdownEditor,
  type MarkdownEditorHandle,
  type MarkdownHeading,
} from "./MarkdownEditor";
import { MediaViewer } from "./MediaViewer";
import { TextViewer, type TextViewerHandle } from "./TextViewer";

type Props = {
  path: string;
  active: boolean;
  showSidebar: boolean;
  revealTarget?: RevealTarget | null;
  onSaveReady: (fn: (() => Promise<void>) | null) => void;
};

export function DocumentTab({ path, active, showSidebar, revealTarget, onSaveReady }: Props) {
  const { t, applied, workspacePath } = useSettings();
  const autoReadonly = applied.general.autoReadonly !== false;
  const [meta, setMeta] = useState<FileMeta | null>(null);
  const [text, setText] = useState<TextFile | null>(null);
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState<"idle" | "saving" | "saved">("idle");
  const [headings, setHeadings] = useState<MarkdownHeading[]>([]);
  const [headingPanelClosed, setHeadingPanelClosed] = useState(false);
  const markdownEditorRef = useRef<MarkdownEditorHandle | null>(null);
  const textViewerRef = useRef<TextViewerHandle | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const [findOpen, setFindOpen] = useState(false);
  const [findQuery, setFindQuery] = useState("");
  const [findModes, setFindModes] = useState<FindModes>(EMPTY_FIND_MODES);
  const [findMatches, setFindMatches] = useState<SearchMatch[]>([]);
  const [findIndex, setFindIndex] = useState(0);
  const [findError, setFindError] = useState("");
  const draftRef = useRef("");
  const pathRef = useRef(path);
  const autoReadonlyRef = useRef(autoReadonly);
  const workspacePathRef = useRef(workspacePath);
  const onSaveReadyRef = useRef(onSaveReady);

  pathRef.current = path;
  draftRef.current = draft;
  autoReadonlyRef.current = autoReadonly;
  workspacePathRef.current = workspacePath;
  onSaveReadyRef.current = onSaveReady;

  const applyLockState = useCallback(async (filePath: string, writable: boolean) => {
    if (!writable || autoReadonlyRef.current) {
      if (pathRef.current === filePath) {
        setEditing(false);
      }
      return;
    }
    try {
      const lock = await AppService.GetFileLock(workspacePathRef.current, filePath);
      if (pathRef.current === filePath) {
        setEditing(lock?.editing ?? true);
      }
    } catch {
      if (pathRef.current === filePath) {
        setEditing(true);
      }
    }
  }, []);

  const load = useCallback(async (filePath: string) => {
    setError("");
    const info = await AppService.Stat(filePath);
    if (pathRef.current !== filePath) {
      return;
    }
    if (!info || !info.exists) {
      setMeta(null);
      setText(null);
      setDraft("");
      setError("missing");
      return;
    }
    setMeta(info);
    await applyLockState(filePath, info.writable);
    if (pathRef.current !== filePath) {
      return;
    }
    if (info.kind === "markdown" || info.kind === "text") {
      const body = await AppService.ReadText(filePath);
      if (pathRef.current !== filePath) {
        return;
      }
      const content = body?.content ?? "";
      setText(body);
      setDraft(content);
      return;
    }
    setText(null);
    setDraft("");
  }, [applyLockState]);

  useEffect(() => {
    load(path).catch((err) => setError(String(err)));
  }, [path, load]);

  useEffect(() => {
    if (!applied.markdown.showHeadingPanel) {
      setHeadingPanelClosed(false);
    }
  }, [applied.markdown.showHeadingPanel]);

  useEffect(() => {
    if (!meta || meta.path !== path) {
      return;
    }
    void applyLockState(path, meta.writable);
  }, [autoReadonly, applyLockState, path, meta]);

  const save = useCallback(async () => {
    const current = pathRef.current;
    const info = meta;
    if (!current || !info || !info.writable || !editing) {
      return;
    }
    if (info.kind !== "markdown" && info.kind !== "text") {
      return;
    }
    setSaving("saving");
    await AppService.WriteText(current, draftRef.current);
    setSaving("saved");
    window.setTimeout(() => setSaving("idle"), 1200);
  }, [editing, meta]);

  useEffect(() => {
    onSaveReadyRef.current(save);
    return () => onSaveReadyRef.current(null);
  }, [save]);

  useEffect(() => {
    if (active) {
      return;
    }
    const focused = document.activeElement;
    if (focused instanceof HTMLElement && rootRef.current?.contains(focused)) {
      focused.blur();
    }
  }, [active]);

  useEffect(() => {
    if (!editing || !meta?.writable) {
      return;
    }
    const timer = window.setTimeout(() => {
      void save();
    }, 700);
    return () => window.clearTimeout(timer);
  }, [draft, editing, meta, save]);

  const canSearch = meta?.kind === "markdown" || meta?.kind === "text";

  const revealMatch = useCallback((match: SearchMatch) => {
    if (meta?.kind === "text") {
      textViewerRef.current?.revealBytes(match.startByte, match.endByte);
      return;
    }
    if (meta?.kind === "markdown") {
      markdownEditorRef.current?.revealMatch(draftRef.current, match);
    }
  }, [meta?.kind]);

  useEffect(() => {
    if (!active || !canSearch) {
      return;
    }
    const openFind = () => {
      setFindOpen(true);
      window.requestAnimationFrame(() => {
        rootRef.current?.querySelector<HTMLInputElement>("[data-vd-find=input]")?.focus();
      });
    };
    window.addEventListener("veda:search-current", openFind);
    return () => window.removeEventListener("veda:search-current", openFind);
  }, [active, canSearch]);

  useEffect(() => {
    if (!findOpen || !canSearch) {
      return;
    }
    const query = findQuery;
    if (!query) {
      setFindMatches([]);
      setFindIndex(0);
      setFindError("");
      return;
    }
    let cancelled = false;
    const timer = window.setTimeout(() => {
      void AppService.SearchContent(draftRef.current, query, searchOptions(findModes, { maxMatches: 2000, maxPerFile: 2000 }))
        .then((result) => {
          if (cancelled) {
            return;
          }
          const matches = result?.files?.[0]?.matches ?? [];
          setFindError("");
          setFindMatches(matches);
          setFindIndex(0);
          if (matches[0]) {
            revealMatch(matches[0]);
          }
        })
        .catch((err) => {
          if (cancelled) {
            return;
          }
          setFindMatches([]);
          setFindError(String(err));
        });
    }, 120);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [findOpen, findQuery, findModes, canSearch, draft, revealMatch]);

  useEffect(() => {
    if (!active || !revealTarget || revealTarget.path !== path) {
      return;
    }
    if (!meta || (meta.kind !== "markdown" && meta.kind !== "text")) {
      return;
    }
    const timer = window.setTimeout(() => {
      revealMatch(revealTarget.match);
    }, 80);
    return () => window.clearTimeout(timer);
  }, [active, revealTarget, path, meta, revealMatch]);

  const stepFind = (delta: number) => {
    if (findMatches.length === 0) {
      return;
    }
    const next = (findIndex + delta + findMatches.length) % findMatches.length;
    setFindIndex(next);
    revealMatch(findMatches[next]!);
  };

  const lockDisabled = !meta?.writable;
  const canToggle = !!meta && (meta.kind === "markdown" || meta.kind === "text") && meta.writable;
  const isMarkdown = meta?.path === path && meta.kind === "markdown";
  const headingPanelAvailable = applied.markdown.showHeadingPanel && isMarkdown;
  const showHeadingPanel = headingPanelAvailable && !headingPanelClosed;
  const showHeadingPanelOpenButton = headingPanelAvailable && headingPanelClosed;
  const titlePad =
    showSidebar || showHeadingPanel ? "" : "pl-[max(1rem,var(--vd-window-controls-inset))]";

  const body = useMemo(() => {
    if (error) {
      return (
        <div className="flex h-full items-center justify-center text-sm text-red-600">
          {error === "missing" ? t("editor.missing") : error}
        </div>
      );
    }
    if (!meta) {
      return <div className="flex h-full items-center justify-center text-sm text-[var(--vd-fg-subtle)]">{t("editor.loading")}</div>;
    }
    if (meta.kind === "markdown" && text) {
      return (
        <MarkdownEditor
          ref={markdownEditorRef}
          path={meta.path}
          dir={meta.dir}
          value={text.content}
          editable={editing && meta.writable}
          onChange={setDraft}
          onHeadingsChange={setHeadings}
        />
      );
    }
    if (meta.kind === "text" && text) {
      return (
        <TextViewer
          ref={textViewerRef}
          value={draft}
          editable={editing && meta.writable}
          onChange={setDraft}
        />
      );
    }
    if (meta.kind === "image" || meta.kind === "svg" || meta.kind === "video") {
      return <MediaViewer path={meta.path} kind={meta.kind} name={meta.name} />;
    }
    return (
      <div className="flex h-full items-center justify-center text-sm text-[var(--vd-fg-muted)]">
        {t("editor.unsupported")}
      </div>
    );
  }, [error, meta, text, editing, draft, t]);

  return (
    <div
      ref={rootRef}
      className="relative flex h-full min-w-0 flex-1 bg-[var(--vd-bg)]"
      aria-hidden={!active}
    >
      {headingPanelAvailable && (
        <HeadingPanel
          headings={headings}
          open={showHeadingPanel}
          t={t}
          workspacePath={workspacePath}
          padForWindowControls={!showSidebar}
          onClose={() => setHeadingPanelClosed(true)}
          onSelect={(index) => markdownEditorRef.current?.scrollToHeading(index)}
        />
      )}
      {showHeadingPanelOpenButton && (
        <button
          type="button"
          tabIndex={active ? 0 : -1}
          className={cn(
            "titlebar-no-drag vd-fade-in absolute top-1/2 z-20 flex h-12 w-4 -translate-y-1/2 items-center justify-center rounded-full border border-[var(--vd-border)] bg-[var(--vd-bg-muted)] text-[var(--vd-fg-subtle)] shadow-sm hover:text-[var(--vd-fg)]",
            showSidebar ? "left-0 -translate-x-1/2" : "left-0",
          )}
          aria-label={t("headingPanel.open")}
          title={t("headingPanel.open")}
          onClick={() => setHeadingPanelClosed(false)}
        >
          <ChevronIcon className="h-3.5 w-3.5" />
        </button>
      )}
      <section className="flex min-w-0 flex-1 flex-col bg-[var(--vd-bg)]">
        <header
          className={cn(
            "titlebar-drag flex h-[var(--vd-titlebar-height)] shrink-0 items-center justify-between border-b border-[var(--vd-border)] bg-[var(--vd-bg)]/90 px-4",
            titlePad,
          )}
        >
          <div className="min-w-0 truncate text-sm font-medium text-[var(--vd-fg)]">
            {meta?.name || "Veda"}
            {saving === "saving" && <span className="ml-2 text-xs font-normal text-[var(--vd-fg-subtle)]">{t("editor.saving")}</span>}
            {saving === "saved" && <span className="ml-2 text-xs font-normal text-[var(--vd-fg-subtle)]">{t("editor.saved")}</span>}
          </div>
          {meta && (meta.kind === "markdown" || meta.kind === "text") && (
            <button
              type="button"
              tabIndex={active ? 0 : -1}
              className={cn(
                "titlebar-no-drag rounded-md p-1.5",
                lockDisabled ? "cursor-not-allowed text-[var(--vd-fg-subtle)]" : "text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
              )}
              title={
                lockDisabled
                  ? t("editor.lockReadonly")
                  : editing
                    ? t("editor.lockOn")
                    : t("editor.lockOff")
              }
              disabled={!canToggle}
              onClick={() => {
                if (!canToggle) {
                  return;
                }
                setEditing((v) => {
                  const next = !v;
                  if (!autoReadonly) {
                    void AppService.SaveFileLock(workspacePath, path, next).catch((err) => {
                      console.error("save file lock failed", err);
                    });
                  }
                  return next;
                });
              }}
            >
              <LockIcon open={editing} className="h-[18px] w-[18px]" />
            </button>
          )}
        </header>
        <div className="relative min-h-0 flex-1">
          {active && findOpen && canSearch && (
            <FindBar
              query={findQuery}
              modes={findModes}
              current={findMatches.length === 0 ? 0 : findIndex + 1}
              total={findMatches.length}
              error={findError}
              t={t}
              onQueryChange={setFindQuery}
              onModesChange={setFindModes}
              onNext={() => stepFind(1)}
              onPrev={() => stepFind(-1)}
              onClose={() => setFindOpen(false)}
            />
          )}
          {body}
        </div>
      </section>
    </div>
  );
}
