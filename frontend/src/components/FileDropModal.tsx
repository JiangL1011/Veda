import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { AppService } from "../lib/api";
import type { FileDropPrompt, FileDropResult } from "../lib/dragdrop";
import {
  FILE_DROP_DONE_EVENT,
  readDragState,
  subscribeDragState,
} from "../lib/dragdrop";
import { fillTemplate } from "../lib/search";
import { useSettings } from "../lib/SettingsContext";
import { baseName, cn, relativePath } from "../lib/utils";
import { CloseIcon, FilePlusIcon, FolderIcon } from "./Icons";

/* 拖拽文件后的选择框：复制、移动，或者取消。
   导入已经执行过之后再出错时，待处理的记录已经用掉了，这时只提示失败原因。 */
export function FileDropModal({ prompt, onClose }: { prompt: FileDropPrompt; onClose: () => void }) {
  const { t } = useSettings();
  const [busy, setBusy] = useState<"copy" | "move" | null>(null);
  const [failures, setFailures] = useState<string[]>([]);
  const busyRef = useRef(false);
  busyRef.current = busy !== null;
  const failed = failures.length > 0;
  const firstRef = useRef<HTMLButtonElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const id = window.requestAnimationFrame(() => firstRef.current?.focus());
    return () => window.cancelAnimationFrame(id);
  }, []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        if (!busyRef.current) {
          onCloseRef.current();
        }
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);

  const files = useMemo(
    () => prompt.files.map((path) => ({ path, name: baseName(path) })),
    [prompt.files],
  );
  const destLabel =
    prompt.destDir === prompt.workspace
      ? baseName(prompt.destDir) || prompt.destDir
      : relativePath(prompt.workspace, prompt.destDir);

  const dismiss = () => {
    if (busyRef.current) {
      return;
    }
    if (!failed) {
      // 取消时要告诉 Go 这次拖拽作废，否则待处理的记录会一直留着。
      void AppService.ResolveFileDrop("cancel").catch(() => undefined);
    }
    onClose();
  };

  const resolve = (action: "copy" | "move") => {
    if (busyRef.current || failed) {
      return;
    }
    setBusy(action);
    setFailures([]);
    void AppService.ResolveFileDrop(action)
      .then((outcome) => {
        const result = outcome as FileDropResult | null;
        if (result) {
          // Go 在预设了复制/移动时才会自己派发这个事件；询问模式下由这里补上，
          // 目录树就能用同一套逻辑刷新。
          window.dispatchEvent(new CustomEvent(FILE_DROP_DONE_EVENT, { detail: result }));
        }
        const errors = result?.errors ?? [];
        if (errors.length > 0) {
          setBusy(null);
          setFailures(errors);
          return;
        }
        onClose();
      })
      .catch((err: unknown) => {
        setBusy(null);
        const reason = err instanceof Error ? err.message.trim() : String(err ?? "").trim();
        setFailures([reason || t("drop.failedTitle")]);
      });
  };

  return createPortal(
    <div className="titlebar-no-drag fixed inset-0 z-[96] flex items-start justify-center p-4 pt-[16vh]">
      <div className="vd-modal-overlay absolute inset-0 bg-[var(--vd-overlay)] backdrop-blur-sm" onMouseDown={dismiss} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="file-drop-title"
        className="vd-modal-panel relative w-[min(520px,94vw)] overflow-hidden rounded-2xl border border-[var(--vd-border)] bg-[var(--vd-bg)] shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between gap-2 border-b border-[var(--vd-border)] px-4 py-3">
          <h2 id="file-drop-title" className="text-sm font-semibold text-[var(--vd-fg)]">
            {t(failed ? "drop.failedTitle" : "drop.title")}
          </h2>
          <button
            type="button"
            aria-label={t("drop.cancel")}
            className="flex h-8 w-8 items-center justify-center rounded-md text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
            onClick={dismiss}
          >
            <CloseIcon className="h-4 w-4" />
          </button>
        </header>
        <div className="px-4 py-4">
          {failed ? (
            <p className="text-sm leading-6 text-[var(--vd-fg)]">
              {fillTemplate(t("drop.failedSummary"), { count: failures.length })}
            </p>
          ) : (
            <p className="text-sm leading-6 text-[var(--vd-fg)]">{t("drop.question")}</p>
          )}
          <div className="mt-3 flex items-center gap-2 text-xs text-[var(--vd-fg-muted)]">
            <FolderIcon className="h-4 w-4 shrink-0" />
            <span className="shrink-0">{t("drop.destination")}</span>
            <span className="min-w-0 flex-1 truncate text-[var(--vd-fg)]" title={prompt.destDir}>
              {destLabel}
            </span>
          </div>
          <ul className="mt-3 max-h-52 overflow-auto rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg-muted)] py-1">
            {files.map((file) => (
              <li key={file.path} className="flex items-center gap-2 px-3 py-1 text-xs text-[var(--vd-fg)]">
                <FilePlusIcon className="h-3.5 w-3.5 shrink-0 text-[var(--vd-fg-muted)]" />
                <span className="min-w-0 flex-1 truncate" title={file.path}>
                  {file.name}
                </span>
              </li>
            ))}
          </ul>
          <p className="mt-2 text-xs text-[var(--vd-fg-subtle)]">
            {fillTemplate(t("drop.fileCount"), { count: files.length })}
          </p>
          {failed && (
            <pre className="mt-3 max-h-40 overflow-auto rounded-md border border-red-500/40 bg-[var(--vd-bg-muted)] px-3 py-2 text-xs leading-5 whitespace-pre-wrap text-red-600">
              {failures.join("\n")}
            </pre>
          )}
        </div>
        <footer className="flex items-center justify-end gap-2 border-t border-[var(--vd-border)] px-4 py-3">
          {failed ? (
            <button
              ref={firstRef}
              type="button"
              className="rounded-lg border border-[var(--vd-fg)] bg-[var(--vd-selected)] px-3 py-1.5 text-sm text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]"
              onClick={onClose}
            >
              {t("drop.close")}
            </button>
          ) : (
            <>
              <button
                type="button"
                disabled={busy !== null}
                className={cn(
                  "rounded-lg border border-[var(--vd-border)] px-3 py-1.5 text-sm",
                  busy
                    ? "cursor-default text-[var(--vd-fg-subtle)]"
                    : "text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]",
                )}
                onClick={dismiss}
              >
                {t("drop.cancel")}
              </button>
              <button
                ref={firstRef}
                type="button"
                disabled={busy !== null}
                className={cn(
                  "rounded-lg border px-3 py-1.5 text-sm",
                  busy === "copy"
                    ? "cursor-default border-[var(--vd-border)] text-[var(--vd-fg-subtle)]"
                    : "border-[var(--vd-border)] text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]",
                )}
                onClick={() => resolve("copy")}
              >
                {t(busy === "copy" ? "drop.working" : "drop.copy")}
              </button>
              <button
                type="button"
                disabled={busy !== null}
                className={cn(
                  "rounded-lg border px-3 py-1.5 text-sm",
                  busy
                    ? "cursor-default border-[var(--vd-border)] text-[var(--vd-fg-subtle)]"
                    : "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]",
                )}
                onClick={() => resolve("move")}
              >
                {t(busy === "move" ? "drop.working" : "drop.move")}
              </button>
            </>
          )}
        </footer>
      </div>
    </div>,
    document.body,
  );
}

/* 预设成复制或移动时不需要询问，但失败项仍然要告诉用户。 */
export function FileDropErrorModal({ errors, onClose }: { errors: string[]; onClose: () => void }) {
  const { t } = useSettings();

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onClose();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose]);

  return createPortal(
    <div className="titlebar-no-drag fixed inset-0 z-[96] flex items-start justify-center p-4 pt-[16vh]">
      <div className="vd-modal-overlay absolute inset-0 bg-[var(--vd-overlay)] backdrop-blur-sm" onMouseDown={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        className="vd-modal-panel relative w-[min(520px,94vw)] overflow-hidden rounded-2xl border border-[var(--vd-border)] bg-[var(--vd-bg)] shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between gap-2 border-b border-[var(--vd-border)] px-4 py-3">
          <h2 className="text-sm font-semibold text-[var(--vd-fg)]">{t("drop.failedTitle")}</h2>
          <button
            type="button"
            aria-label={t("drop.close")}
            className="flex h-8 w-8 items-center justify-center rounded-md text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
            onClick={onClose}
          >
            <CloseIcon className="h-4 w-4" />
          </button>
        </header>
        <div className="px-4 py-4">
          <p className="text-sm leading-6 text-[var(--vd-fg)]">
            {fillTemplate(t("drop.failedSummary"), { count: errors.length })}
          </p>
          <pre className="mt-3 max-h-60 overflow-auto rounded-md border border-red-500/40 bg-[var(--vd-bg-muted)] px-3 py-2 text-xs leading-5 whitespace-pre-wrap text-red-600">
            {errors.join("\n")}
          </pre>
        </div>
        <footer className="flex items-center justify-end border-t border-[var(--vd-border)] px-4 py-3">
          <button
            type="button"
            className="rounded-lg border border-[var(--vd-fg)] bg-[var(--vd-selected)] px-3 py-1.5 text-sm text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]"
            onClick={onClose}
          >
            {t("drop.close")}
          </button>
        </footer>
      </div>
    </div>,
    document.body,
  );
}

export function DragDropOverlay() {
  const { t } = useSettings();
  const [state, setState] = useState(readDragState);

  useEffect(() => {
    setState(readDragState());
    return subscribeDragState(() => setState(readDragState()));
  }, []);

  if (!state.active) {
    return null;
  }
  const label = state.label.trim();
  return createPortal(
    <div className="vd-drop-shade pointer-events-none fixed inset-0 z-[70] flex items-start justify-center pt-[12vh]">
      <div className="vd-drop-hint flex max-w-[80vw] items-center gap-2 rounded-full border border-[var(--vd-border)] bg-[var(--vd-bg)] px-4 py-2 text-sm text-[var(--vd-fg)] shadow-xl">
        <FilePlusIcon className="h-4 w-4 shrink-0 text-[var(--vd-fg-muted)]" />
        <span className="truncate">
          {label ? fillTemplate(t("drop.hint"), { target: label }) : t("drop.hintGeneric")}
        </span>
      </div>
    </div>,
    document.body,
  );
}
