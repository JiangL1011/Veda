import { useCallback, useEffect, useLayoutEffect, useRef, useState, type PointerEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { FileEntry } from "../../bindings/veda/models";
import { AppService } from "../lib/api";
import { useSettings } from "../lib/SettingsContext";
import { cn, copyText, hostPlatform, isSameOrInside, relativePath } from "../lib/utils";
import { ConfirmDeleteModal, type DeleteMode } from "./ConfirmDeleteModal";
import { SettingsButton } from "./SettingsModal";
import {
  ChevronIcon,
  FileTypeIcon,
  FilePlusIcon,
  FolderIcon,
  FolderPlusIcon,
  MarkdownIcon,
} from "./Icons";

function sortEntries(rows: FileEntry[] | null | undefined) {
  return [...(rows ?? [])].sort((a, b) => {
    if (a.isDir !== b.isDir) {
      return a.isDir ? -1 : 1;
    }
    return a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
  });
}

function KindIcon({ name, kind, isDir }: { name: string; kind: string; isDir: boolean }) {
  return (
    <FileTypeIcon
      name={name}
      kind={kind}
      isDir={isDir}
      className="h-4 w-4 shrink-0 text-[var(--vd-fg-muted)]"
    />
  );
}

function NameInput({
  defaultName,
  selectStem,
  onCancel,
  onSubmit,
}: {
  defaultName: string;
  selectStem: boolean;
  onCancel: () => void;
  onSubmit: (name: string) => void | Promise<void>;
}) {
  const ref = useRef<HTMLInputElement>(null);
  const submitted = useRef(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    el.focus();
    const stem = selectStem ? defaultName.replace(/\.[^./\\]+$/, "") : defaultName;
    el.setSelectionRange(0, stem.length);
  }, [defaultName, selectStem]);

  const commit = () => {
    if (submitted.current) {
      return;
    }
    const value = ref.current?.value.trim() ?? "";
    if (!value) {
      onCancel();
      return;
    }
    submitted.current = true;
    void Promise.resolve(onSubmit(value)).catch(() => {
      submitted.current = false;
      ref.current?.focus();
    });
  };

  return (
    <input
      ref={ref}
      defaultValue={defaultName}
      spellCheck={false}
      className="min-w-0 flex-1 rounded border border-[var(--vd-border)] bg-[var(--vd-bg)] px-1 py-0.5 text-[13px] leading-5 text-[var(--vd-fg)] outline-none ring-1 ring-[var(--vd-border)]"
      onClick={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()}
      onKeyDown={(e) => {
        e.stopPropagation();
        if (e.key === "Enter") {
          e.preventDefault();
          commit();
        }
        if (e.key === "Escape") {
          e.preventDefault();
          submitted.current = true;
          onCancel();
        }
      }}
      onBlur={commit}
    />
  );
}

function CreateNameInput({
  depth,
  kind,
  onCancel,
  onSubmit,
}: {
  depth: number;
  kind: "folder" | "markdown";
  onCancel: () => void;
  onSubmit: (name: string) => void;
}) {
  const { t } = useSettings();
  const defaultName = kind === "folder" ? t("sidebar.defaultFolder") : t("sidebar.defaultMarkdown");

  return (
    <div
      className="flex items-center gap-1.5 py-1 pr-2"
      style={{ paddingLeft: 8 + depth * 14 }}
    >
      <span className="w-3.5" />
      {kind === "folder" ? (
        <FolderIcon className="h-4 w-4 shrink-0 text-stone-500" />
      ) : (
        <MarkdownIcon className="h-4 w-4 shrink-0 text-stone-500" />
      )}
      <NameInput defaultName={defaultName} selectStem onCancel={onCancel} onSubmit={onSubmit} />
    </div>
  );
}

function ActionIconButton({
  title,
  onClick,
  children,
}: {
  title: string;
  onClick: () => void;
  children: ReactNode;
}) {
  const btnRef = useRef<HTMLButtonElement>(null);
  const tipRef = useRef<HTMLSpanElement>(null);
  const [tip, setTip] = useState<{ left: number; top: number } | null>(null);

  const showTip = () => {
    const rect = btnRef.current?.getBoundingClientRect();
    if (!rect) {
      return;
    }
    setTip({ left: rect.left + rect.width / 2, top: rect.top - 6 });
  };

  useLayoutEffect(() => {
    const el = tipRef.current;
    if (!tip || !el) {
      return;
    }
    el.style.left = `${Math.round(tip.left - el.offsetWidth / 2)}px`;
    el.style.top = `${Math.round(tip.top - el.offsetHeight)}px`;
  }, [tip]);

  return (
    <button
      ref={btnRef}
      type="button"
      aria-label={title}
      className="relative flex h-6 w-6 items-center justify-center rounded text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)] hover:text-[var(--vd-fg)]"
      onMouseEnter={showTip}
      onMouseLeave={() => setTip(null)}
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        setTip(null);
        onClick();
      }}
    >
      {children}
      {tip &&
        createPortal(
          <span
            ref={tipRef}
            className="pointer-events-none fixed z-50 rounded bg-[var(--vd-fg)] px-2 py-1 text-xs leading-none whitespace-nowrap text-[var(--vd-bg)]"
          >
            {title}
          </span>,
          document.body,
        )}
    </button>
  );
}

function TreeContextMenu({
  x,
  y,
  canDelete,
  onClose,
  onRename,
  onReveal,
  onCopyName,
  onCopyRel,
  onCopyAbs,
  onCopyFile,
  onDelete,
}: {
  x: number;
  y: number;
  canDelete: boolean;
  onClose: () => void;
  onRename: () => void;
  onReveal: () => void;
  onCopyName: () => void;
  onCopyRel: () => void;
  onCopyAbs: () => void;
  onCopyFile: () => void;
  onDelete: () => void;
}) {
  const { t } = useSettings();
  const ref = useRef<HTMLDivElement>(null);
  const platform = hostPlatform();
  const revealKey =
    platform === "mac" ? "sidebar.reveal.mac" : platform === "windows" ? "sidebar.reveal.windows" : "sidebar.reveal.linux";

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    const pad = 8;
    const rect = el.getBoundingClientRect();
    let left = x;
    let top = y;
    if (left + rect.width > window.innerWidth - pad) {
      left = Math.max(pad, window.innerWidth - rect.width - pad);
    }
    if (top + rect.height > window.innerHeight - pad) {
      top = Math.max(pad, window.innerHeight - rect.height - pad);
    }
    el.style.left = `${Math.round(left)}px`;
    el.style.top = `${Math.round(top)}px`;
  }, [x, y]);

  useEffect(() => {
    const onDown = (event: MouseEvent) => {
      if (ref.current?.contains(event.target as Node)) {
        return;
      }
      onClose();
    };
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      }
    };
    window.addEventListener("mousedown", onDown, true);
    window.addEventListener("keydown", onKey, true);
    window.addEventListener("scroll", onClose, true);
    window.addEventListener("resize", onClose);
    return () => {
      window.removeEventListener("mousedown", onDown, true);
      window.removeEventListener("keydown", onKey, true);
      window.removeEventListener("scroll", onClose, true);
      window.removeEventListener("resize", onClose);
    };
  }, [onClose]);

  const item = (label: string, action: () => void, danger = false) => (
    <button
      type="button"
      role="menuitem"
      className={cn(
        "flex w-full px-3 py-1.5 text-left text-[13px] leading-5",
        danger ? "text-red-600 hover:bg-[var(--vd-hover)]" : "text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]",
      )}
      onClick={() => {
        onClose();
        action();
      }}
    >
      {label}
    </button>
  );

  return createPortal(
    <div
      ref={ref}
      role="menu"
      className="titlebar-no-drag fixed z-[80] min-w-[188px] overflow-hidden rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] py-1 shadow-lg"
      style={{ left: x, top: y }}
      onMouseDown={(e) => e.stopPropagation()}
      onContextMenu={(e) => {
        e.preventDefault();
        e.stopPropagation();
      }}
    >
      {item(t("sidebar.rename"), onRename)}
      {item(t(revealKey), onReveal)}
      {item(t("sidebar.copyName"), onCopyName)}
      {item(t("sidebar.copyRelPath"), onCopyRel)}
      {item(t("sidebar.copyAbsPath"), onCopyAbs)}
      {item(t("sidebar.copyFile"), onCopyFile)}
      {canDelete && (
        <>
          <div className="my-1 h-px bg-[var(--vd-border)]" />
          {item(t("sidebar.delete"), onDelete, true)}
        </>
      )}
    </div>,
    document.body,
  );
}

function TreeNode({
  entry,
  depth,
  rootPath,
  activePath,
  hiddenResourcePath,
  onOpen,
  onRename,
  onDelete,
}: {
  entry: FileEntry;
  depth: number;
  rootPath: string;
  activePath?: string;
  hiddenResourcePath?: string;
  onOpen: (entry: FileEntry) => void;
  onRename: (path: string, name: string) => Promise<FileEntry | null>;
  onDelete: (path: string, mode: DeleteMode) => Promise<boolean>;
}) {
  const { t } = useSettings();
  const [open, setOpen] = useState(depth === 0);
  const [children, setChildren] = useState<FileEntry[] | null>(entry.isDir ? null : []);
  const [creating, setCreating] = useState<"folder" | "markdown" | null>(null);
  const [renaming, setRenaming] = useState(false);
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const rowRef = useRef<HTMLDivElement>(null);

  const loadChildren = async () => {
    const rows = await AppService.ReadDir(entry.path);
    setChildren(sortEntries(rows));
  };

  const prevPath = useRef(entry.path);

  useEffect(() => {
    if (prevPath.current === entry.path) {
      return;
    }
    prevPath.current = entry.path;
    setChildren(entry.isDir ? null : []);
    setCreating(null);
    setRenaming(false);
    setMenu(null);
    setConfirmingDelete(false);
  }, [entry.path, entry.isDir]);

  useEffect(() => {
    if (!entry.isDir || !open || children) {
      return;
    }
    loadChildren().catch(() => setChildren([]));
  }, [entry.isDir, entry.path, open, children]);

  const holdsActive =
    entry.isDir && !!activePath && activePath !== entry.path && isSameOrInside(entry.path, activePath);

  useEffect(() => {
    if (holdsActive) {
      setOpen(true);
    }
  }, [holdsActive]);

  const startCreate = (kind: "folder" | "markdown") => {
    setCreating(kind);
    setOpen(true);
    if (children === null) {
      loadChildren().catch(() => setChildren([]));
    }
  };

  const finishCreate = async (kind: "folder" | "markdown", name: string) => {
    try {
      const created =
        kind === "folder"
          ? await AppService.CreateFolder(entry.path, name)
          : await AppService.CreateMarkdown(entry.path, name);
      setCreating(null);
      await loadChildren();
      if (created && !created.isDir) {
        onOpen(created);
      }
    } catch {
      setCreating(null);
    }
  };

  const handleRename = async (path: string, name: string) => {
    const next = await onRename(path, name);
    if (next) {
      setChildren((rows) => {
        if (!rows?.some((row) => row.path === path)) {
          return rows;
        }
        return sortEntries(rows.map((row) => (row.path === path ? next : row)));
      });
    }
    return next;
  };

  const handleDeleteChild = async (path: string, mode: DeleteMode) => {
    const ok = await onDelete(path, mode);
    if (ok) {
      setChildren((rows) => rows?.filter((row) => row.path !== path) ?? rows);
    }
    return ok;
  };

  const finishRename = async (name: string) => {
    const next = await handleRename(entry.path, name);
    if (!next) {
      throw new Error("rename failed");
    }
    setRenaming(false);
  };

  const startRename = () => {
    setCreating(null);
    setMenu(null);
    setRenaming(true);
  };

  const closeMenu = useCallback(() => setMenu(null), []);

  const runDelete = async (mode: DeleteMode) => {
    const ok = await onDelete(entry.path, mode);
    if (ok) {
      setConfirmingDelete(false);
    }
    return ok;
  };

  const selected = !entry.isDir && activePath === entry.path;

  useEffect(() => {
    if (selected) {
      rowRef.current?.scrollIntoView({ block: "nearest" });
    }
  }, [selected]);

  return (
    <div>
      <div
        ref={rowRef}
        className={cn(
          "group/row relative flex w-full items-center text-[13px] leading-5 select-none",
          selected ? "bg-[var(--vd-selected)] text-[var(--vd-fg)]" : "text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
          !selected && menu && "bg-[var(--vd-hover)]",
        )}
        onContextMenu={(e) => {
          e.preventDefault();
          e.stopPropagation();
          if (renaming) {
            return;
          }
          setMenu({ x: e.clientX, y: e.clientY });
        }}
      >
        <div
          className={cn(
            "flex min-w-0 flex-1 items-center gap-1.5 py-1 pr-2",
            entry.isDir && !renaming && "pr-14",
          )}
          style={{ paddingLeft: 8 + depth * 14 }}
        >
          {entry.isDir ? (
            <button
              type="button"
              className="-ml-0.5 flex h-5 w-5 shrink-0 items-center justify-center text-[var(--vd-fg-subtle)]"
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                if (renaming) {
                  return;
                }
                setOpen((v) => !v);
              }}
            >
              <ChevronIcon open={open} className="h-3.5 w-3.5" />
            </button>
          ) : (
            <span className="w-3.5 shrink-0" />
          )}
          {renaming ? (
            <>
              <KindIcon name={entry.name} kind={entry.kind} isDir={entry.isDir} />
              <NameInput
                defaultName={entry.name}
                selectStem={!entry.isDir}
                onCancel={() => setRenaming(false)}
                onSubmit={finishRename}
              />
            </>
          ) : (
            <button
              type="button"
              className="flex min-w-0 flex-1 items-center gap-1.5 text-left"
              onClick={() => {
                if (entry.isDir) {
                  setOpen((v) => !v);
                  return;
                }
                onOpen(entry);
              }}
            >
              <KindIcon name={entry.name} kind={entry.kind} isDir={entry.isDir} />
              <span className="truncate">{entry.name}</span>
            </button>
          )}
        </div>
        {entry.isDir && !renaming && (
          <div className="absolute top-1/2 right-1 flex -translate-y-1/2 items-center opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100">
            <ActionIconButton title={t("sidebar.newFolder")} onClick={() => startCreate("folder")}>
              <FolderPlusIcon className="h-3.5 w-3.5" />
            </ActionIconButton>
            <ActionIconButton title={t("sidebar.newMarkdown")} onClick={() => startCreate("markdown")}>
              <FilePlusIcon className="h-3.5 w-3.5" />
            </ActionIconButton>
          </div>
        )}
      </div>
      {menu && (
        <TreeContextMenu
          x={menu.x}
          y={menu.y}
          canDelete={depth > 0}
          onClose={closeMenu}
          onRename={startRename}
          onReveal={() => void AppService.RevealInFileManager(entry.path).catch((err) => console.error(err))}
          onCopyName={() => void copyText(entry.name)}
          onCopyRel={() => void copyText(relativePath(rootPath, entry.path))}
          onCopyAbs={() => void copyText(entry.path)}
          onCopyFile={() => void AppService.CopyFile(entry.path).catch((err) => console.error(err))}
          onDelete={() => setConfirmingDelete(true)}
        />
      )}
      {confirmingDelete && (
        <ConfirmDeleteModal
          name={entry.name}
          path={entry.path}
          isDir={entry.isDir}
          onCancel={() => setConfirmingDelete(false)}
          onConfirm={runDelete}
        />
      )}
      {entry.isDir && open && (
        <div>
          {creating && (
            <CreateNameInput
              depth={depth + 1}
              kind={creating}
              onCancel={() => setCreating(null)}
              onSubmit={(name) => void finishCreate(creating, name)}
            />
          )}
          {children?.filter((child) => child.path !== hiddenResourcePath).map((child) => (
            <TreeNode
              key={child.path}
              entry={child}
              depth={depth + 1}
              rootPath={rootPath}
              activePath={activePath}
              hiddenResourcePath={hiddenResourcePath}
              onOpen={onOpen}
              onRename={handleRename}
              onDelete={handleDeleteChild}
            />
          ))}
        </div>
      )}
    </div>
  );
}

type Props = {
  rootPath: string;
  rootName: string;
  activePath?: string;
  onOpen: (entry: FileEntry) => void;
  onRename: (path: string, newName: string) => Promise<FileEntry | null>;
  onDelete: (path: string, mode: DeleteMode) => Promise<boolean>;
};

const SIDEBAR_DEFAULT_WIDTH = 256;
const SIDEBAR_MIN_WIDTH = 180;
const SIDEBAR_MAX_WIDTH = 480;

function clampSidebarWidth(width: number) {
  const room = typeof window === "undefined" ? SIDEBAR_MAX_WIDTH : window.innerWidth - 360;
  const max = Math.max(SIDEBAR_MIN_WIDTH, Math.min(SIDEBAR_MAX_WIDTH, room));
  return Math.min(max, Math.max(SIDEBAR_MIN_WIDTH, Math.round(width)));
}

export function Sidebar({ rootPath, rootName, activePath, onOpen, onRename, onDelete }: Props) {
  const { applied } = useSettings();
  const [width, setWidth] = useState(SIDEBAR_DEFAULT_WIDTH);
  const [dragging, setDragging] = useState(false);
  const widthRef = useRef(width);
  widthRef.current = width;

  const root: FileEntry = {
    name: rootName,
    path: rootPath,
    isDir: true,
    kind: "dir",
  };
  const resourceParts = applied.general.resourceDirectory
    .trim()
    .split(/[/\\]+/)
    .filter((part) => part && part !== ".");
  const pathSeparator = rootPath.includes("\\") && !rootPath.includes("/") ? "\\" : "/";
  const hiddenResourcePath = applied.general.showResourceDirectory
    ? undefined
    : [rootPath.replace(/[/\\]+$/, ""), ...resourceParts].join(pathSeparator);

  useLayoutEffect(() => {
    let cancelled = false;
    AppService.GetLayout(rootPath)
      .then((layout) => {
        if (cancelled) {
          return;
        }
        const rec = (layout ?? {}) as Record<string, unknown>;
        const next = Number(rec.sidebarWidth ?? rec.SidebarWidth ?? 0);
        if (next > 0) {
          setWidth(clampSidebarWidth(next));
        }
      })
      .catch(() => {
        /* 保持默认行为 */
      });
    return () => {
      cancelled = true;
      document.documentElement.classList.remove("vd-sidebar-resizing");
    };
  }, [rootPath]);

  const persistWidth = (next: number) => {
    const clamped = clampSidebarWidth(next);
    setWidth(clamped);
    void AppService.SaveSidebarWidth(rootPath, clamped).catch((err) => {
      console.error("save sidebar width failed", err);
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
      setWidth(clampSidebarWidth(startWidth + (move.clientX - startX)));
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
      className={cn(
        "relative flex h-full shrink-0 flex-col border-r border-[var(--vd-border)] bg-[var(--vd-bg-muted)]",
        dragging && "select-none",
      )}
      style={{ width }}
    >
      <div className="titlebar-drag h-[var(--vd-titlebar-height)] shrink-0" />
      <div
        className="titlebar-no-drag min-h-0 flex-1 overflow-auto py-1"
        onContextMenu={(e) => e.preventDefault()}
      >
        <TreeNode
          entry={root}
          depth={0}
          rootPath={rootPath}
          activePath={activePath}
          hiddenResourcePath={hiddenResourcePath}
          onOpen={onOpen}
          onRename={onRename}
          onDelete={onDelete}
        />
      </div>
      <div className="titlebar-no-drag border-t border-[var(--vd-border)] p-2">
        <SettingsButton />
      </div>
      <div
        role="separator"
        aria-orientation="vertical"
        aria-label="Resize sidebar"
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
    </aside>
  );
}
