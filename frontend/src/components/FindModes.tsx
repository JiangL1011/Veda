import { cn } from "../lib/utils";

export function FindModeButton({
  pressed,
  title,
  label,
  onClick,
}: {
  pressed: boolean;
  title: string;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      data-vd-find="mode"
      title={title}
      aria-pressed={pressed}
      className={cn(
        "flex h-6 min-w-6 items-center justify-center rounded px-1 font-mono text-[11px] font-semibold",
        pressed
          ? "bg-[var(--vd-selected)] text-[var(--vd-fg)]"
          : "text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]",
      )}
      onClick={onClick}
    >
      {label}
    </button>
  );
}

export function FindModesRow({
  modes,
  t,
  onChange,
}: {
  modes: { caseSensitive: boolean; wholeWord: boolean; useRegex: boolean };
  t: (key: string) => string;
  onChange: (next: { caseSensitive: boolean; wholeWord: boolean; useRegex: boolean }) => void;
}) {
  return (
    <div className="flex items-center gap-0.5">
      <FindModeButton
        pressed={modes.caseSensitive}
        title={t("search.matchCase")}
        label="Aa"
        onClick={() => onChange({ ...modes, caseSensitive: !modes.caseSensitive })}
      />
      <FindModeButton
        pressed={modes.wholeWord}
        title={t("search.wholeWord")}
        label="W"
        onClick={() => onChange({ ...modes, wholeWord: !modes.wholeWord })}
      />
      <FindModeButton
        pressed={modes.useRegex}
        title={t("search.regex")}
        label=".*"
        onClick={() => onChange({ ...modes, useRegex: !modes.useRegex })}
      />
    </div>
  );
}
