import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { SearchMatch } from "../../bindings/veda/models";
import { AppService } from "../lib/api";
import {
  fillTemplate,
  searchOptions,
  type WorkspaceSearchSession,
} from "../lib/search";
import { cn } from "../lib/utils";
import { FindModesRow } from "./FindModes";
import { CloseIcon } from "./Icons";

export type WorkspaceJump = {
  path: string;
  match: SearchMatch;
};

type Props = {
  workspacePath: string | null;
  t: (key: string) => string;
  session: WorkspaceSearchSession;
  onSessionChange: (next: WorkspaceSearchSession) => void;
  onClose: () => void;
  onJump: (target: WorkspaceJump) => void;
};

function searchKey(workspacePath: string | null, query: string, modes: WorkspaceSearchSession["modes"]) {
  return `${workspacePath ?? ""}\0${query}\0${modes.caseSensitive ? 1 : 0}${modes.wholeWord ? 1 : 0}${modes.useRegex ? 1 : 0}`;
}

export function WorkspaceSearchModal({
  workspacePath,
  t,
  session,
  onSessionChange,
  onClose,
  onJump,
}: Props) {
  const [query, setQuery] = useState(session.query);
  const [modes, setModes] = useState(session.modes);
  const [files, setFiles] = useState(session.files);
  const [error, setError] = useState(session.error);
  const [truncated, setTruncated] = useState(session.truncated);
  const [active, setActive] = useState(session.active);
  const [searched, setSearched] = useState(session.searched);
  const inputRef = useRef<HTMLInputElement | null>(null);
  const requestRef = useRef<{ cancel?: () => void } | null>(null);
  const sessionRef = useRef(onSessionChange);
  sessionRef.current = onSessionChange;
  const completedKey = useRef(session.searched ? searchKey(workspacePath, session.query, session.modes) : "");

  const flat = useMemo(() => {
    const rows: { file: (typeof files)[number]; match: SearchMatch; index: number }[] = [];
    for (const file of files) {
      for (const match of file.matches ?? []) {
        rows.push({ file, match, index: rows.length });
      }
    }
    return rows;
  }, [files]);

  useEffect(() => {
    sessionRef.current({ query, modes, files, error, truncated, active, searched });
  }, [query, modes, files, error, truncated, active, searched]);

  useEffect(() => {
    const id = window.requestAnimationFrame(() => {
      const el = inputRef.current;
      if (!el) {
        return;
      }
      el.focus();
      el.select();
    });
    return () => window.cancelAnimationFrame(id);
  }, []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  useEffect(() => {
    requestRef.current?.cancel?.();
    if (!workspacePath) {
      setFiles([]);
      setError("");
      setTruncated(false);
      setSearched(false);
      return;
    }
    const q = query.trim();
    const key = searchKey(workspacePath, query, modes);
    if (!q) {
      setFiles([]);
      setError("");
      setTruncated(false);
      setSearched(false);
      completedKey.current = key;
      return;
    }
    if (completedKey.current === key) {
      return;
    }
    let cancelled = false;
    const handle = window.setTimeout(() => {
      const pending = AppService.SearchWorkspace(
        workspacePath,
        q,
        searchOptions(modes, { contextLines: 1, maxMatches: 800, maxPerFile: 50 }),
      );
      requestRef.current = { cancel: () => pending.cancel() };
      void pending
        .then((result) => {
          if (cancelled) {
            return;
          }
          completedKey.current = key;
          setError("");
          setFiles(result?.files ?? []);
          setTruncated(!!result?.truncated);
          setActive(0);
          setSearched(true);
        })
        .catch((err) => {
          if (cancelled) {
            return;
          }
          completedKey.current = key;
          setFiles([]);
          setError(String(err));
          setSearched(true);
        });
    }, 220);
    return () => {
      cancelled = true;
      window.clearTimeout(handle);
      requestRef.current?.cancel?.();
    };
  }, [query, modes, workspacePath]);

  const jump = (target: WorkspaceJump) => {
    onJump(target);
    onClose();
  };

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setActive((prev) => Math.min(Math.max(flat.length - 1, 0), prev + 1));
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setActive((prev) => Math.max(0, prev - 1));
      }
      if (event.key === "Enter" && flat[active]) {
        event.preventDefault();
        jump({ path: flat[active]!.file.path, match: flat[active]!.match });
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [flat, active, onJump, onClose]);

  return createPortal(
    <div className="titlebar-no-drag fixed inset-0 z-[80] flex items-start justify-center p-4 pt-[10vh] sm:p-8 sm:pt-[10vh]">
      <button type="button" className="absolute inset-0 bg-[var(--vd-overlay)] backdrop-blur-md" aria-label={t("search.close")} onClick={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="workspace-search-title"
        className="relative flex max-h-[min(720px,78vh)] w-[min(760px,94vw)] flex-col overflow-hidden rounded-2xl border border-[var(--vd-border)] bg-[var(--vd-bg)] shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="flex items-center gap-2 border-b border-[var(--vd-border)] px-4 py-3">
          <h2 id="workspace-search-title" className="absolute h-px w-px overflow-hidden">
            {t("search.workspaceTitle")}
          </h2>
          <input
            ref={inputRef}
            data-vd-find="workspace"
            value={query}
            placeholder={t("search.workspacePlaceholder")}
            spellCheck={false}
            className="h-9 min-w-0 flex-1 bg-transparent text-sm text-[var(--vd-fg)] outline-none placeholder:text-[var(--vd-placeholder)]"
            onChange={(e) => setQuery(e.target.value)}
          />
          <FindModesRow modes={modes} t={t} onChange={setModes} />
          <button
            type="button"
            aria-label={t("search.close")}
            className="flex h-8 w-8 items-center justify-center rounded-md text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
            onClick={onClose}
          >
            <CloseIcon className="h-4 w-4" />
          </button>
        </header>
        <div className="min-h-0 flex-1 overflow-auto px-2 py-2">
          {!workspacePath && (
            <p className="px-3 py-8 text-center text-sm text-[var(--vd-fg-muted)]">{t("search.workspaceNeedFolder")}</p>
          )}
          {workspacePath && !query.trim() && (
            <p className="px-3 py-8 text-center text-sm text-[var(--vd-fg-muted)]">{t("search.workspaceEmpty")}</p>
          )}
          {workspacePath && query.trim() && error && (
            <p className="px-3 py-8 text-center text-sm text-red-600">{t("search.invalidRegex")}</p>
          )}
          {workspacePath && query.trim() && !error && files.length === 0 && (
            <p className="px-3 py-8 text-center text-sm text-[var(--vd-fg-muted)]">{t("search.workspaceNone")}</p>
          )}
          {files.map((file) => (
            <section key={file.path} className="mb-2">
              <div className="flex items-baseline justify-between gap-2 px-2 py-1">
                <div className="truncate text-[13px] font-medium text-[var(--vd-fg)]">{file.relPath || file.path}</div>
                <div className="shrink-0 text-[11px] text-[var(--vd-fg-subtle)]">
                  {fillTemplate(t("search.matchesInFile"), { count: file.matches?.length ?? 0 })}
                </div>
              </div>
              {(file.matches ?? []).map((match, i) => {
                const index = flat.findIndex((row) => row.file.path === file.path && row.match === match);
                const selected = index === active;
                return (
                  <button
                    key={`${file.path}:${match.startByte}:${i}`}
                    type="button"
                    className={cn(
                      "mb-0.5 block w-full rounded-md px-2 py-1.5 text-left",
                      selected ? "bg-[var(--vd-selected)]" : "hover:bg-[var(--vd-hover)]",
                    )}
                    onMouseEnter={() => setActive(index)}
                    onClick={() => jump({ path: file.path, match })}
                  >
                    <MatchPreview match={match} />
                  </button>
                );
              })}
            </section>
          ))}
          {truncated && (
            <p className="px-3 py-2 text-center text-[11px] text-[var(--vd-fg-subtle)]">{t("search.truncated")}</p>
          )}
        </div>
      </div>
    </div>,
    document.body,
  );
}

function MatchPreview({ match }: { match: SearchMatch }) {
  const before = (match.before ?? []).join("\n");
  const after = (match.after ?? []).join("\n");
  const line = match.lineText ?? "";
  const col = Math.max(0, (match.column || 1) - 1);
  const endCol =
    match.endLine === match.line ? Math.max(col, (match.endColumn || col + match.match.length) - 1) : col + match.match.length;
  return (
    <pre className="whitespace-pre-wrap font-mono text-[12px] leading-5 text-[var(--vd-fg-muted)]">
      {before ? <span>{before}{"\n"}</span> : null}
      <span>
        {line.slice(0, col)}
        <mark className="vd-search-hit-inline">{line.slice(col, endCol)}</mark>
        {line.slice(endCol)}
      </span>
      {after ? <span>{"\n"}{after}</span> : null}
    </pre>
  );
}
