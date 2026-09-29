import { useEffect, useRef } from "react";
import { fillTemplate } from "../lib/search";
import { cn } from "../lib/utils";
import { FindModesRow } from "./FindModes";
import { ArrowDownIcon, ArrowUpIcon, CloseIcon } from "./Icons";

type Modes = { caseSensitive: boolean; wholeWord: boolean; useRegex: boolean };

type Props = {
  query: string;
  modes: Modes;
  current: number;
  total: number;
  error?: string;
  t: (key: string) => string;
  onQueryChange: (next: string) => void;
  onModesChange: (next: Modes) => void;
  onNext: () => void;
  onPrev: () => void;
  onClose: () => void;
};

export function FindBar({
  query,
  modes,
  current,
  total,
  error,
  t,
  onQueryChange,
  onModesChange,
  onNext,
  onPrev,
  onClose,
}: Props) {
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  return (
    <div
      data-vd-find="file"
      className="titlebar-no-drag pointer-events-auto absolute top-2 right-3 z-30 flex items-center gap-1 rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-1.5 py-1 shadow-lg"
    >
      <input
        ref={inputRef}
        data-vd-find="input"
        value={query}
        placeholder={t("search.findPlaceholder")}
        spellCheck={false}
        className="h-7 w-44 bg-transparent px-1.5 text-[13px] text-[var(--vd-fg)] outline-none placeholder:text-[var(--vd-placeholder)]"
        onChange={(e) => onQueryChange(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            if (e.shiftKey) {
              onPrev();
            } else {
              onNext();
            }
          }
          if (e.key === "Escape") {
            e.preventDefault();
            onClose();
          }
        }}
      />
      <span className={cn("min-w-12 px-1 text-center text-[11px] tabular-nums text-[var(--vd-fg-subtle)]")}>
        {error
          ? t("search.invalidRegex")
          : total === 0
            ? t("search.noResults")
            : fillTemplate(t("search.resultCount"), { current, total })}
      </span>
      <FindModesRow modes={modes} t={t} onChange={onModesChange} />
      <button
        type="button"
        data-vd-find="prev"
        title={t("search.prev")}
        className="flex h-6 w-6 items-center justify-center rounded text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
        onClick={onPrev}
      >
        <ArrowUpIcon className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        data-vd-find="next"
        title={t("search.next")}
        className="flex h-6 w-6 items-center justify-center rounded text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
        onClick={onNext}
      >
        <ArrowDownIcon className="h-3.5 w-3.5" />
      </button>
      <button
        type="button"
        data-vd-find="close"
        title={t("search.close")}
        className="flex h-6 w-6 items-center justify-center rounded text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
        onClick={onClose}
      >
        <CloseIcon className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
