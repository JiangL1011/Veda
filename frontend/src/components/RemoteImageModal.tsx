import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { cn } from "../lib/utils";
import { CloseIcon } from "./Icons";

type Props = {
  t: (key: string) => string;
  onCancel: () => void;
  onSubmit: (url: string) => void;
};

function isImageURL(value: string) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

export function RemoteImageModal({ t, onCancel, onSubmit }: Props) {
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement | null>(null);
  const onCancelRef = useRef(onCancel);
  onCancelRef.current = onCancel;

  useEffect(() => {
    const id = window.requestAnimationFrame(() => inputRef.current?.focus());
    return () => window.cancelAnimationFrame(id);
  }, []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onCancelRef.current();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);

  const submit = () => {
    const value = url.trim();
    if (!value) {
      return;
    }
    if (!isImageURL(value)) {
      setError(t("editor.remoteImageInvalid"));
      return;
    }
    onSubmit(value);
  };

  return createPortal(
    <div className="titlebar-no-drag fixed inset-0 z-[90] flex items-start justify-center p-4 pt-[18vh]">
      <div className="absolute inset-0 bg-[var(--vd-overlay)] backdrop-blur-sm" onMouseDown={onCancel} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="remote-image-title"
        className="relative w-[min(520px,94vw)] overflow-hidden rounded-2xl border border-[var(--vd-border)] bg-[var(--vd-bg)] shadow-2xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between gap-2 border-b border-[var(--vd-border)] px-4 py-3">
          <h2 id="remote-image-title" className="text-sm font-semibold text-[var(--vd-fg)]">
            {t("editor.importRemoteImage")}
          </h2>
          <button
            type="button"
            aria-label={t("settings.close")}
            className="flex h-8 w-8 items-center justify-center rounded-md text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
            onClick={onCancel}
          >
            <CloseIcon className="h-4 w-4" />
          </button>
        </header>
        <div className="px-4 py-4">
          <label className="block text-xs text-[var(--vd-fg-muted)]" htmlFor="remote-image-url">
            {t("editor.remoteImagePrompt")}
          </label>
          <input
            id="remote-image-url"
            ref={inputRef}
            value={url}
            type="url"
            spellCheck={false}
            placeholder="https://example.com/picture.png"
            className="mt-2 h-9 w-full rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-3 text-sm text-[var(--vd-fg)] outline-none placeholder:text-[var(--vd-placeholder)] focus:border-[var(--vd-fg-muted)]"
            onChange={(e) => {
              setUrl(e.target.value);
              setError("");
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                submit();
              }
            }}
          />
          {error && <p className="mt-2 text-xs leading-5 text-red-600">{error}</p>}
        </div>
        <footer className="flex items-center justify-end gap-2 border-t border-[var(--vd-border)] px-4 py-3">
          <button
            type="button"
            className="rounded-lg border border-[var(--vd-border)] px-3 py-1.5 text-sm text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
            onClick={onCancel}
          >
            {t("editor.remoteImageCancel")}
          </button>
          <button
            type="button"
            disabled={!url.trim()}
            className={cn(
              "rounded-lg border px-3 py-1.5 text-sm",
              url.trim()
                ? "border-[var(--vd-fg)] bg-[var(--vd-selected)] text-[var(--vd-fg)]"
                : "cursor-default border-[var(--vd-border)] text-[var(--vd-fg-subtle)]",
            )}
            onClick={submit}
          >
            {t("editor.remoteImageConfirm")}
          </button>
        </footer>
      </div>
    </div>,
    document.body,
  );
}
