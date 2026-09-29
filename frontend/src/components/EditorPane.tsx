import { useCallback, useEffect, useRef, useState } from "react";
import { AppService } from "../lib/api";
import { useSettings } from "../lib/SettingsContext";
import {
  closeDocTab,
  dropDocTabs,
  NO_DOC_TABS,
  openDocTab,
  pruneDocTabs,
  restoreDocTabs,
  rewriteDocPath,
  rewriteDocTabs,
  type DocTabs,
} from "../lib/docTabs";
import { cn } from "../lib/utils";
import { DocumentTab } from "./DocumentTab";
import { ChevronIcon, CloseIcon, FileTypeIcon } from "./Icons";
import type { RevealTarget } from "../lib/search";

type Props = {
  path: string | null;
  showSidebar: boolean;
  workspacePath?: string;
  pathRewrite?: { from: string; to: string } | null;
  forgetPath?: string | null;
  revealTarget?: RevealTarget | null;
  onDirtySaveRef?: (fn: () => Promise<void>) => void;
  onCloseTabRef?: (fn: () => boolean) => void;
  onPathChange: (path: string | null) => void;
};

function rewriteKeys<T>(map: Map<string, T>, from: string, to: string): Map<string, T> {
  const next = new Map<string, T>();
  for (const [key, value] of map) {
    next.set(rewriteDocPath(key, from, to), value);
  }
  return next;
}

function fileName(path: string) {
  const parts = path.split(/[/\\]/);
  return parts[parts.length - 1] || path;
}

function StripArrow({
  label,
  flip,
  disabled,
  onClick,
}: {
  label: string;
  flip?: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled}
      className={cn(
        "flex h-8 w-6 shrink-0 items-center justify-center rounded-md text-[var(--vd-fg-muted)]",
        disabled ? "cursor-default opacity-30" : "hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]",
      )}
      onClick={onClick}
    >
      <ChevronIcon className={cn("h-4 w-4", flip && "rotate-180")} />
    </button>
  );
}

export function EditorPane({
  path,
  showSidebar,
  workspacePath = "",
  pathRewrite,
  forgetPath,
  revealTarget,
  onDirtySaveRef,
  onCloseTabRef,
  onPathChange,
}: Props) {
  const { t, applied } = useSettings();
  const [tabs, setTabs] = useState<DocTabs>(NO_DOC_TABS);
  const [loaded, setLoaded] = useState<string[]>([]);
  const [tabsReady, setTabsReady] = useState(!showSidebar);
  const savesRef = useRef(new Map<string, () => Promise<void>>());
  const pathRef = useRef(path);
  pathRef.current = path;
  const maxTabs = applied.general.maxDocTabs;
  const singleRow = applied.general.docTabsLayout !== "multi";
  const stripRef = useRef<HTMLDivElement | null>(null);
  const [canScroll, setCanScroll] = useState({ start: false, end: false });

  const forgetLoaded = (gone: string[]) => {
    if (gone.length === 0) {
      return;
    }
    setLoaded((prev) => prev.filter((item) => !gone.includes(item)));
    for (const item of gone) {
      void savesRef.current.get(item)?.();
      savesRef.current.delete(item);
    }
  };

  const prune = useCallback(() => {
    setTabs((prev) => {
      const { tabs: next, dropped } = pruneDocTabs(prev, pathRef.current ?? "", maxTabs);
      forgetLoaded(dropped);
      return next;
    });
  }, [maxTabs]);

  useEffect(() => {
    if (!showSidebar) {
      setTabsReady(true);
      return;
    }
    let cancelled = false;
    setTabsReady(false);
    (async () => {
      const layout = await AppService.GetLayout(workspacePath).catch(() => null);
      if (cancelled) {
        return;
      }
      const rec = (layout ?? {}) as Record<string, unknown>;
      const saved = Array.isArray(rec.openTabs)
        ? rec.openTabs.filter((item): item is string => typeof item === "string")
        : Array.isArray(rec.OpenTabs)
          ? rec.OpenTabs.filter((item): item is string => typeof item === "string")
          : [];
      const storedActive = typeof rec.activeTab === "string" ? rec.activeTab : typeof rec.ActiveTab === "string" ? rec.ActiveTab : "";
      const current = pathRef.current ?? "";
      const restored = restoreDocTabs(saved, current || storedActive);
      const active = restored.order.includes(current)
        ? current
        : restored.order.includes(storedActive)
          ? storedActive
          : (restored.order[0] ?? "");
      setTabs(active ? pruneDocTabs(restored, active, maxTabs).tabs : restored);
      setLoaded(active ? [active] : []);
      setTabsReady(true);
      if (active !== current) {
        onPathChange(active || null);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [showSidebar, workspacePath]);

  useEffect(() => {
    if (!tabsReady || !path) {
      return;
    }
    setLoaded((prev) => (prev.includes(path) ? prev : [...prev, path]));
    setTabs((prev) => {
      const opened = openDocTab(prev, path);
      const { tabs: next, dropped } = pruneDocTabs(opened, path, maxTabs);
      forgetLoaded(dropped);
      return next;
    });
  }, [path, maxTabs, tabsReady]);

  useEffect(() => {
    prune();
  }, [prune]);

  useEffect(() => {
    const from = pathRewrite?.from;
    const to = pathRewrite?.to;
    if (!from || !to) {
      return;
    }
    savesRef.current = rewriteKeys(savesRef.current, from, to);
    setLoaded((prev) => [...new Set(prev.map((item) => rewriteDocPath(item, from, to)))]);
    setTabs((prev) => rewriteDocTabs(prev, from, to));
  }, [pathRewrite]);

  useEffect(() => {
    if (!forgetPath) {
      return;
    }
    setTabs((prev) => {
      const next = dropDocTabs(prev, forgetPath);
      forgetLoaded(prev.order.filter((item) => !next.order.includes(item)));
      if (pathRef.current && !next.order.includes(pathRef.current)) {
        onPathChange(next.recent[0] ?? null);
      }
      return next;
    });
  }, [forgetPath, onPathChange]);

  useEffect(() => {
    if (!showSidebar || !workspacePath || !tabsReady) {
      return;
    }
    void AppService.SaveOpenTabs(workspacePath, tabs.order, path ?? "").catch((err) => {
      console.error("save open tabs failed", err);
    });
  }, [showSidebar, workspacePath, tabsReady, tabs.order, path]);

  const saveActive = useCallback(async () => {
    const current = pathRef.current;
    if (!current) {
      return;
    }
    await savesRef.current.get(current)?.();
  }, []);

  useEffect(() => {
    onDirtySaveRef?.(saveActive);
  }, [onDirtySaveRef, saveActive]);

  const closeTab = useCallback(
    (tabPath: string) => {
      void savesRef.current.get(tabPath)?.();
      savesRef.current.delete(tabPath);
      const next = closeDocTab(tabs, tabPath);
      setTabs(next);
      setLoaded((prev) => prev.filter((item) => item !== tabPath));
      if (tabPath === pathRef.current) {
        onPathChange(next.recent[0] ?? null);
      }
    },
    [onPathChange, tabs],
  );

  /* 单文件窗口没有标签栏，没有标签页时也无从关起：两种情况都返回 false，让上层
     去关闭窗口。 */
  const closeActiveTab = useCallback(() => {
    const current = pathRef.current;
    if (!showSidebar || !current) {
      return false;
    }
    closeTab(current);
    return true;
  }, [closeTab, showSidebar]);

  useEffect(() => {
    onCloseTabRef?.(closeActiveTab);
  }, [onCloseTabRef, closeActiveTab]);

  const syncScrollEdges = useCallback(() => {
    const strip = stripRef.current;
    if (!strip) {
      return;
    }
    const max = strip.scrollWidth - strip.clientWidth;
    setCanScroll({ start: strip.scrollLeft > 1, end: strip.scrollLeft < max - 1 });
  }, []);

  useEffect(() => {
    const strip = stripRef.current;
    if (!strip) {
      setCanScroll({ start: false, end: false });
      return;
    }
    syncScrollEdges();
    const observer = new ResizeObserver(syncScrollEdges);
    observer.observe(strip);
    return () => observer.disconnect();
  }, [syncScrollEdges, singleRow, tabs.order]);

  useEffect(() => {
    const strip = stripRef.current;
    if (!strip || !singleRow || !path) {
      return;
    }
    const tab = strip.children[tabs.order.indexOf(path)];
    if (!(tab instanceof HTMLElement)) {
      return;
    }
    const right = tab.offsetLeft + tab.offsetWidth;
    if (tab.offsetLeft < strip.scrollLeft) {
      strip.scrollTo({ left: tab.offsetLeft, behavior: "smooth" });
    } else if (right > strip.scrollLeft + strip.clientWidth) {
      strip.scrollTo({ left: right - strip.clientWidth, behavior: "smooth" });
    }
  }, [path, singleRow, tabs.order]);

  const showArrows = singleRow && (canScroll.start || canScroll.end);

  const scrollStrip = (direction: -1 | 1) => {
    const strip = stripRef.current;
    if (!strip) {
      return;
    }
    strip.scrollBy({ left: direction * Math.max(120, strip.clientWidth * 0.8), behavior: "smooth" });
  };

  const empty = (
    <div className="flex h-full items-center justify-center text-sm text-[var(--vd-fg-subtle)]">
      {t("editor.empty")}
    </div>
  );

  if (!path && tabs.order.length === 0) {
    return <div className="relative h-full min-h-0 min-w-0 flex-1 bg-[var(--vd-bg)]">{empty}</div>;
  }

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-[var(--vd-bg)]">
      {showSidebar && tabs.order.length > 0 && (
        <div className="titlebar-no-drag flex shrink-0 items-center gap-1 border-b border-[var(--vd-border)] bg-[var(--vd-bg-muted)] px-1">
          {showArrows && (
            <StripArrow
              label={t("tabs.scrollLeft")}
              flip
              disabled={!canScroll.start}
              onClick={() => scrollStrip(-1)}
            />
          )}
          <div
            ref={stripRef}
            onScroll={syncScrollEdges}
            className={cn(
              /* 滚动条只是被隐藏，Shift + 滚轮依然能横向滚动。 */
              "vd-hide-scrollbar flex min-w-0 flex-1 gap-1 py-1",
              singleRow
                ? "flex-nowrap overflow-x-auto overflow-y-hidden"
                : "flex-wrap content-start overflow-visible",
            )}
          >
            {tabs.order.map((tabPath) => {
              const active = tabPath === path;
              return (
                <div
                  key={tabPath}
                  role="tab"
                  tabIndex={0}
                  aria-selected={active}
                  title={tabPath}
                  className={cn(
                    "group flex h-8 min-w-[120px] max-w-[220px] shrink-0 cursor-pointer items-center gap-2 rounded-md border px-2.5 text-left text-xs transition-colors",
                    active
                      ? "border-[var(--vd-border)] bg-[var(--vd-bg)] text-[var(--vd-fg)] shadow-sm"
                      : "border-transparent text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]",
                  )}
                  onClick={() => onPathChange(tabPath)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" || event.key === " ") {
                      event.preventDefault();
                      onPathChange(tabPath);
                    }
                  }}
                >
                  <FileTypeIcon
                    name={fileName(tabPath)}
                    className="h-3.5 w-3.5 shrink-0 opacity-70"
                  />
                  <span className="min-w-0 flex-1 truncate">{fileName(tabPath)}</span>
                  <button
                    type="button"
                    aria-label={t("tabs.close")}
                    className="flex h-5 w-5 shrink-0 cursor-pointer items-center justify-center rounded opacity-50 hover:bg-[var(--vd-hover)] hover:opacity-100"
                    onClick={(event) => {
                      event.stopPropagation();
                      void closeTab(tabPath);
                    }}
                  >
                    <CloseIcon className="h-3 w-3" />
                  </button>
                </div>
              );
            })}
          </div>
          {showArrows && (
            <StripArrow
              label={t("tabs.scrollRight")}
              disabled={!canScroll.end}
              onClick={() => scrollStrip(1)}
            />
          )}
        </div>
      )}
      <div className="relative min-h-0 min-w-0 flex-1">
        {!path && empty}
        {tabs.order.filter((tabPath) => loaded.includes(tabPath)).map((tabPath) => {
          const active = tabPath === path;
          return (
            <div
              key={tabPath}
              /* 每个标签页都占据同一个绝对定位的盒子，这样切换标签只是显示/隐藏的
                 切换，而不是会压缩其滚动位置的重新布局。 */
              className={cn(
                "absolute inset-0 flex min-h-0 min-w-0",
                !active && "pointer-events-none invisible",
              )}
            >
              <DocumentTab
                path={tabPath}
                active={active}
                showSidebar={showSidebar}
                revealTarget={active ? revealTarget : null}
                onSaveReady={(fn) => {
                  if (fn) {
                    savesRef.current.set(tabPath, fn);
                  } else {
                    savesRef.current.delete(tabPath);
                  }
                }}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
