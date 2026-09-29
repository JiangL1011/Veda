import { useLayoutEffect, useRef, useState, type PointerEvent } from "react";
import { AppService } from "../lib/api";
import { cn } from "../lib/utils";
import { CloseIcon } from "./Icons";
import type { MarkdownHeading } from "./MarkdownEditor";

type Props = {
  headings: MarkdownHeading[];
  open: boolean;
  onClose: () => void;
  onSelect: (index: number) => void;
  t: (key: string) => string;
  padForWindowControls?: boolean;
  workspacePath: string;
};

const HEADING_PANEL_DEFAULT_WIDTH = 224;
const HEADING_PANEL_MIN_WIDTH = 160;
const HEADING_PANEL_MAX_WIDTH = 560;

function clampHeadingPanelWidth(width: number) {
  const room = typeof window === "undefined" ? HEADING_PANEL_MAX_WIDTH : window.innerWidth - 360;
  const max = Math.max(HEADING_PANEL_MIN_WIDTH, Math.min(HEADING_PANEL_MAX_WIDTH, room));
  return Math.min(max, Math.max(HEADING_PANEL_MIN_WIDTH, Math.round(width)));
}

export function HeadingPanel({
  headings,
  open,
  onClose,
  onSelect,
  t,
  padForWindowControls,
  workspacePath,
}: Props) {
  const [width, setWidth] = useState(HEADING_PANEL_DEFAULT_WIDTH);
  const [dragging, setDragging] = useState(false);
  const widthRef = useRef(width);
  widthRef.current = width;

  useLayoutEffect(() => {
    let cancelled = false;
    if (!workspacePath) {
      return () => {
        cancelled = true;
        document.documentElement.classList.remove("vd-sidebar-resizing");
      };
    }
    AppService.GetLayout(workspacePath)
      .then((layout) => {
        if (cancelled) {
          return;
        }
        const rec = (layout ?? {}) as Record<string, unknown>;
        const next = Number(rec.headingPanelWidth ?? rec.HeadingPanelWidth ?? 0);
        if (next > 0) {
          setWidth(clampHeadingPanelWidth(next));
        }
      })
      .catch(() => {
        /* 保持默认行为 */
      });
    return () => {
      cancelled = true;
      document.documentElement.classList.remove("vd-sidebar-resizing");
    };
  }, [workspacePath]);

  const persistWidth = (next: number) => {
    const clamped = clampHeadingPanelWidth(next);
    setWidth(clamped);
    if (!workspacePath) {
      return;
    }
    void AppService.SaveHeadingPanelWidth(workspacePath, clamped).catch((err) => {
      console.error("save heading panel width failed", err);
    });
  };

  const onResizePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    const handle = event.currentTarget;
    handle.setPointerCapture(event.pointerId);
    setDragging(true);
    document.documentElement.classList.add("vd-sidebar-resizing");
    const startX = event.clientX;
    const startWidth = widthRef.current;

    const onMove = (move: globalThis.PointerEvent) => {
      setWidth(clampHeadingPanelWidth(startWidth + (move.clientX - startX)));
    };
    const onUp = (up: globalThis.PointerEvent) => {
      handle.releasePointerCapture(up.pointerId);
      handle.removeEventListener("pointermove", onMove);
      handle.removeEventListener("pointerup", onUp);
      handle.removeEventListener("pointercancel", onUp);
      document.documentElement.classList.remove("vd-sidebar-resizing");
      setDragging(false);
      persistWidth(startWidth + (up.clientX - startX));
    };
    handle.addEventListener("pointermove", onMove);
    handle.addEventListener("pointerup", onUp);
    handle.addEventListener("pointercancel", onUp);
  };

  return (
    <aside
      aria-hidden={!open}
      className={cn(
        "relative h-full shrink-0 bg-[var(--vd-bg-subtle)]",
        open && "border-r border-[var(--vd-border)]",
        dragging ? "select-none" : "transition-[width] duration-200 ease-out",
      )}
      style={{ width: open ? width : 0 }}
    >
      <div className="h-full overflow-hidden">
        <div className="flex h-full flex-col" style={{ width }}>
          <header
            className={cn(
              "titlebar-drag flex h-[var(--vd-titlebar-height)] shrink-0 items-center justify-between border-b border-[var(--vd-border)] pr-3",
              padForWindowControls ? "pl-[max(0.75rem,var(--vd-window-controls-inset))]" : "pl-3",
            )}
          >
            <h2 className="truncate text-xs font-semibold tracking-wide text-[var(--vd-fg-muted)] uppercase">
              {t("headingPanel.title")}
            </h2>
            <button
              type="button"
              tabIndex={open ? 0 : -1}
              className="titlebar-no-drag flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-[var(--vd-fg-subtle)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
              aria-label={t("headingPanel.close")}
              title={t("headingPanel.close")}
              onClick={onClose}
            >
              <CloseIcon className="h-3.5 w-3.5" />
            </button>
          </header>
          <nav className="min-h-0 flex-1 overflow-y-auto px-2 py-2" aria-label={t("headingPanel.title")}>
            {headings.length === 0 ? (
              <p className="px-2 py-2 text-xs leading-5 text-[var(--vd-fg-subtle)]">{t("headingPanel.empty")}</p>
            ) : (
              headings.map((heading, index) => (
                <button
                  key={`${index}-${heading.level}-${heading.text}`}
                  type="button"
                  tabIndex={open ? 0 : -1}
                  className="block w-full truncate rounded-md py-1.5 pr-2 text-left text-[13px] text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
                  style={{ paddingLeft: `${8 + (heading.level - 1) * 12}px` }}
                  title={heading.text}
                  onClick={() => onSelect(index)}
                >
                  {heading.text}
                </button>
              ))
            )}
          </nav>
        </div>
      </div>
      {open && (
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label={t("headingPanel.resize")}
          className="titlebar-no-drag absolute inset-y-0 -right-1 z-20 w-2 cursor-col-resize touch-none"
          onPointerDown={onResizePointerDown}
        >
          <span
            className={cn(
              "absolute inset-y-0 left-1/2 w-px -translate-x-1/2 bg-transparent transition-colors",
              dragging ? "bg-[var(--vd-fg-subtle)]" : "hover:bg-[var(--vd-fg-subtle)]",
            )}
          />
        </div>
      )}
    </aside>
  );
}
