import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { fillTemplate } from "../lib/search";
import { useSettings } from "../lib/SettingsContext";
import { cn } from "../lib/utils";
import { CloseIcon } from "./Icons";

export type DeleteMode = "trash" | "delete";

type Props = {
  name: string;
  path: string;
  isDir: boolean;
  onCancel: () => void;
  onConfirm: (mode: DeleteMode) => Promise<boolean>;
};

export function ConfirmDeleteModal({ name, path, isDir, onCancel, onConfirm }: Props) {
  const { t } = useSettings();
  const [busy, setBusy] = useState<DeleteMode | null>(null);
  const [error, setError] = useState("");
  const trashRef = useRef<HTMLButtonElement>(null);
  const busyRef = useRef(false);
  busyRef.current = busy !== null;
  const onCancelRef = useRef(onCancel);
  onCancelRef.current = onCancel;

  useEffect(() => {
    const id = window.requestAnimationFrame(() => trashRef.current?.focus());
    return () => window.cancelAnimationFrame(id);
  }, []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        if (!busyRef.current) {
          onCancelRef.current();
        }
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);

  const confirm = (mode: DeleteMode) => {
    if (busy) {
      return;
    }
    setBusy(mode);
    setError("");
    const fallback = mode === "trash" ? "sidebar.trashFailed" : "sidebar.deleteFailed";
    void onConfirm(mode)
      .then((ok) => {
        if (!ok) {
          setBusy(null);
          setError(t(fallback));
        }
      })
      .catch((err: unknown) => {
        setBusy(null);
        const reason = err instanceof Error ? err.message.trim() : String(err ?? "").trim();
        setError(reason || t(fallback));
      });
  };

  const dismiss = () => {
    if (!busy) {
      onCancel();
    }
  };

  return createPortal(
    <div className="titlebar-no-drag fixed inset-0 z-[95] flex items-start justify-center p-4 pt-[18vh]">
      <div className="absolute inset-0 bg-[var(--vd-overlay)] backdrop-blur-sm" onMouseDown={dismiss} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-delete-title"
        className="relative w-[min(460px,94vw)] overflow-hidden rounded-2xl border border-[var(--vd-border)] bg-[var(--vd-bg)] shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between gap-2 border-b border-[var(--vd-border)] px-4 py-3">
          <h2 id="confirm-delete-title" className="text-sm font-semibold text-[var(--vd-fg)]">
            {t(isDir ? "sidebar.deleteFolderTitle" : "sidebar.deleteFileTitle")}
          </h2>
          <button
            type="button"
            aria-label={t("settings.close")}
            className="flex h-8 w-8 items-center justify-center rounded-md text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
            onClick={dismiss}
          >
            <CloseIcon className="h-4 w-4" />
          </button>
        </header>
        <div className="px-4 py-4">
          <p className="text-sm leading-6 text-[var(--vd-fg)]">
            {fillTemplate(t(isDir ? "sidebar.deleteConfirmFolder" : "sidebar.deleteConfirmFile"), { name })}
          </p>
          <p className="mt-1 truncate text-xs text-[var(--vd-fg-subtle)]" title={path}>
            {path}
          </p>
          {isDir && (
            <p className="mt-3 rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg-muted)] px-3 py-2 text-xs leading-5 text-[var(--vd-fg-muted)]">
              {t("sidebar.deleteFolderHint")}
            </p>
          )}
          <p className="mt-3 text-xs leading-5 text-[var(--vd-fg-muted)]">{t("sidebar.deleteIrreversible")}</p>
          {error && <p className="mt-2 text-xs leading-5 text-red-600">{error}</p>}
        </div>
        <footer className="flex items-center justify-end gap-2 border-t border-[var(--vd-border)] px-4 py-3">
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
            {t("sidebar.deleteCancel")}
          </button>
          <button
            ref={trashRef}
            type="button"
            disabled={busy !== null}
            className={cn(
              "rounded-lg border px-3 py-1.5 text-sm",
              busy
                ? "cursor-default border-[var(--vd-border)] text-[var(--vd-fg-subtle)]"
                : "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]",
            )}
            onClick={() => confirm("trash")}
          >
            {t(busy === "trash" ? "sidebar.trashing" : "sidebar.moveToTrash")}
          </button>
          <button
            type="button"
            disabled={busy !== null}
            className={cn(
              "rounded-lg border px-3 py-1.5 text-sm",
              busy
                ? "cursor-default border-[var(--vd-border)] text-[var(--vd-fg-subtle)]"
                : "border-red-600 bg-red-600 text-white hover:bg-red-700",
            )}
            onClick={() => confirm("delete")}
          >
            {t(busy === "delete" ? "sidebar.deleting" : "sidebar.deleteConfirm")}
          </button>
        </footer>
      </div>
    </div>,
    document.body,
  );
}
