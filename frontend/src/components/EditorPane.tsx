import { useCallback, useEffect, useRef, useState } from "react";
import { Window } from "@wailsio/runtime";
import { AppService } from "../lib/api";
import { useSettings } from "../lib/SettingsContext";
import {
  closeDocTab,
  dropDocTabs,
  moveDocTab,
  NO_DOC_TABS,
  openDocTab,
  pruneDocTabs,
  restoreDocTabs,
  rewriteDocPath,
  rewriteDocTabs,
  type DocTabs,
} from "../lib/docTabs";
import { cn, isSameOrInside } from "../lib/utils";
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
  onSaveTreeRef?: (fn: (path: string) => Promise<void>) => void;
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
  onSaveTreeRef,
  onCloseTabRef,
  onPathChange,
}: Props) {
  const { t, applied } = useSettings();
  const [tabs, setTabs] = useState<DocTabs>(NO_DOC_TABS);
  /* 已挂载内容面板的路径，按首次打开的顺序排列；标签栏排序不影响它。 */
  const [loaded, setLoaded] = useState<string[]>([]);
  const [tabsReady, setTabsReady] = useState(!showSidebar);
  const savesRef = useRef(new Map<string, () => Promise<void>>());
  const pathRef = useRef(path);
  pathRef.current = path;
  const maxTabs = applied.general.maxDocTabs;
  const singleRow = applied.general.docTabsLayout !== "multi";
  const stripRef = useRef<HTMLDivElement | null>(null);
  const [canScroll, setCanScroll] = useState({ start: false, end: false });
  const [draggingTab, setDraggingTab] = useState<string | null>(null);
  const tabsRef = useRef(tabs);
  tabsRef.current = tabs;
  const tabDragRef = useRef<{ path: string; startX: number; startY: number; active: boolean } | null>(null);
  const suppressTabClickRef = useRef(false);

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
    if (!showSidebar || !workspacePath || !tabsReady || draggingTab) {
      return;
    }
    void AppService.SaveOpenTabs(workspacePath, tabs.order, path ?? "").catch((err) => {
      console.error("save open tabs failed", err);
    });
  }, [showSidebar, workspacePath, tabsReady, tabs.order, path, draggingTab]);

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

  /* 移动或重命名目录树里的条目之前，先把落在这个路径下的所有已打开文档
     保存到旧位置，避免路径改写让标签页重新挂载时丢掉未落盘的草稿。 */
  const saveTree = useCallback(async (root: string) => {
    if (!root) {
      return;
    }
    const targets = [...savesRef.current.keys()].filter((item) => isSameOrInside(root, item));
    await Promise.all(targets.map((item) => savesRef.current.get(item)?.()));
  }, []);

  useEffect(() => {
    onSaveTreeRef?.(saveTree);
  }, [onSaveTreeRef, saveTree]);

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

  /* 标签排序：标签本身是 no-drag 区域，按下后移动超过阈值就进入排序，
     不会启动窗口拖动。指针位置按标签的实际矩形换算成插入槽位。 */
  const beginTabDrag = (event: React.PointerEvent<HTMLDivElement>, tabPath: string) => {
    /* 新的一次按下意味着上一次拖拽补发的 click 已经处理完，可以复位拦截标记。 */
    suppressTabClickRef.current = false;
    if (event.button !== 0 || tabsRef.current.order.length < 2) {
      return;
    }
    if ((event.target as Element | null)?.closest("button")) {
      return;
    }
    tabDragRef.current = { path: tabPath, startX: event.clientX, startY: event.clientY, active: false };
  };

  useEffect(() => {
    const dropSlot = (x: number, y: number, dragPath: string) => {
      const strip = stripRef.current;
      const others = tabsRef.current.order.filter((item) => item !== dragPath);
      if (!strip) {
        return others.length;
      }
      const elements = new Map<string, HTMLElement>();
      for (const element of strip.querySelectorAll<HTMLElement>("[data-tab-path]")) {
        const path = element.dataset.tabPath;
        if (path) {
          elements.set(path, element);
        }
      }
      /* 多行布局也按行优先比较：指针在某一行里就按水平中点判断，
         到了下一行上方则插到那个标签之前。 */
      for (let i = 0; i < others.length; i++) {
        const element = elements.get(others[i]!);
        if (!element) {
          continue;
        }
        const rect = element.getBoundingClientRect();
        if (y < rect.top || (y <= rect.bottom && x < rect.left + rect.width / 2)) {
          return i;
        }
      }
      return others.length;
    };

    const onMove = (event: PointerEvent) => {
      const drag = tabDragRef.current;
      if (!drag) {
        return;
      }
      if (!drag.active) {
        if (Math.abs(event.clientX - drag.startX) < 4 && Math.abs(event.clientY - drag.startY) < 4) {
          return;
        }
        drag.active = true;
        setDraggingTab(drag.path);
        document.documentElement.classList.add("vd-tab-dragging");
      }
      event.preventDefault();
      const slot = dropSlot(event.clientX, event.clientY, drag.path);
      const next = moveDocTab(tabsRef.current, drag.path, slot);
      if (next !== tabsRef.current) {
        tabsRef.current = next;
        setTabs(next);
      }
    };

    const finish = () => {
      const drag = tabDragRef.current;
      if (!drag) {
        return;
      }
      tabDragRef.current = null;
      if (!drag.active) {
        return;
      }
      setDraggingTab(null);
      document.documentElement.classList.remove("vd-tab-dragging");
      /* 拖拽结束后浏览器还会补一个 click，别让它顺带切换或关闭标签页。 */
      suppressTabClickRef.current = true;
    };

    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", finish);
    window.addEventListener("pointercancel", finish);
    window.addEventListener("blur", finish);
    return () => {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", finish);
      window.removeEventListener("pointercancel", finish);
      window.removeEventListener("blur", finish);
      document.documentElement.classList.remove("vd-tab-dragging");
    };
  }, []);

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
    if (!strip || !singleRow || !path || draggingTab) {
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
  }, [path, singleRow, tabs.order, draggingTab]);

  const showArrows = singleRow && (canScroll.start || canScroll.end);
  /* 有标签时整条标签栏就是窗口标题栏：空白处可拖动窗口，双击最大化。
     macOS 的双击由 Wails 的原生拖拽区处理，这里只给其他平台兜底；
     标签本身和两端的箭头不参与，避免双击标签时误最大化。 */
  const showTabStrip = showSidebar && tabs.order.length > 0;

  const maximizeFromTabStrip = (event: React.MouseEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement | null;
    if (target?.closest("[role='tab'], button")) {
      return;
    }
    void Window.ToggleMaximise().catch(() => undefined);
  };

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

  /* 拖拽落点：编辑区对应工作区根目录。Go 只用到 data-drop-dir，
     另外两个属性用于拖拽过程中的界面提示。 */
  const dropTarget = {
    "data-file-drop-target": "editor",
    "data-drop-dir": workspacePath,
    "data-drop-label": workspacePath ? fileName(workspacePath) : "",
  };

  if (!path && tabs.order.length === 0) {
    return (
      <div className="relative h-full min-h-0 min-w-0 flex-1 bg-[var(--vd-bg)]" {...dropTarget}>
        {/* 没有标签栏时，顶部仍然留一条标题栏拖拽区（窗口顶部不再有原生拖拽）。 */}
        <div className="titlebar-drag absolute inset-x-0 top-0 h-[var(--vd-titlebar-height)]" />
        {empty}
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col bg-[var(--vd-bg)]" {...dropTarget}>
      {showTabStrip && (
        <div
          className="titlebar-drag flex shrink-0 select-none items-center gap-1 border-b border-[var(--vd-border)] bg-[var(--vd-bg-muted)] px-1"
          onDoubleClick={maximizeFromTabStrip}
        >
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
              /* 滚动条只是被隐藏，Shift + 滚轮依然能横向滚动。标签栏空白处
                 和标签之间的间隙都属于这个拖拽区；标签自身是 no-drag。 */
              "titlebar-drag vd-hide-scrollbar flex min-w-0 flex-1 gap-1 py-1",
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
                  data-tab-path={tabPath}
                  className={cn(
                    "titlebar-no-drag group flex h-8 min-w-[120px] max-w-[220px] shrink-0 cursor-pointer select-none items-center gap-2 rounded-md border px-2.5 text-left text-xs transition-colors",
                    active
                      ? "border-[var(--vd-border)] bg-[var(--vd-bg)] text-[var(--vd-fg)] shadow-sm"
                      : "border-transparent text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]",
                    draggingTab === tabPath && "opacity-60",
                  )}
                  onPointerDown={(event) => beginTabDrag(event, tabPath)}
                  onClick={() => {
                    if (suppressTabClickRef.current) {
                      return;
                    }
                    onPathChange(tabPath);
                  }}
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
                      if (suppressTabClickRef.current) {
                        return;
                      }
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
        {/* 面板按首次打开的顺序渲染，不跟随标签栏排序：React 移动带 key 的
            DOM 节点会重置编辑器里的滚动位置。 */}
        {loaded
          .filter((tabPath) => tabs.order.includes(tabPath))
          .map((tabPath) => {
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
                  titlebarDrag={!showTabStrip}
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
