import { useCallback, useEffect, useLayoutEffect, useRef, useState, type DragEvent as ReactDragEvent, type PointerEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { FileEntry } from "../../bindings/veda/models";
import { AppService } from "../lib/api";
import {
  FILE_DROP_DONE_EVENT,
  setTreeDragActive,
  TREE_DRAG_MIME,
  TREE_MOVE_DONE_EVENT,
  type FileDropResult,
  type TreeMoveResult,
} from "../lib/dragdrop";
import { useSettings } from "../lib/SettingsContext";
import { baseName, cn, copyText, dirName, hostPlatform, isSameOrInside, relativePath } from "../lib/utils";
import { ConfirmDeleteModal, type DeleteMode } from "./ConfirmDeleteModal";
import { SettingsButton } from "./SettingsModal";
import { UpdateButton } from "./UpdateButton";
import {
  ChevronIcon,
  FileTypeIcon,
  FilePlusIcon,
  FolderIcon,
  FolderPlusIcon,
} from "./Icons";

function sortEntries(rows: FileEntry[] | null | undefined) {
  return [...(rows ?? [])].sort((a, b) => {
    if (a.isDir !== b.isDir) {
      return a.isDir ? -1 : 1;
    }
    return a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
  });
}

type Translate = (key: string) => string;

/* 后端会为需要区分处理的失败带上 "[code] 文案" 前缀，这里取出 code，
   换成当前语言的提示；其余错误保持原样展示。 */
const ERROR_CODE = /^\[([a-z0-9-]+)\]\s*/;

function backendError(err: unknown) {
  const raw = (err instanceof Error ? err.message : String(err ?? "")).trim();
  const match = ERROR_CODE.exec(raw);
  if (!match) {
    return { code: "", text: raw };
  }
  return { code: match[1], text: raw.slice(match[0].length).trim() };
}

function createErrorMessage(err: unknown, t: Translate) {
  const { code, text } = backendError(err);
  if (code === "unsupported-doc-type") {
    return t("sidebar.docTypeUnsupported");
  }
  return text || t("sidebar.createFailed");
}

function renameErrorMessage(err: unknown, t: Translate) {
  const { code, text } = backendError(err);
  if (code === "non-text-extension") {
    return t("sidebar.nonTextRenameUnsupported");
  }
  if (code === "unsupported-doc-type") {
    return t("sidebar.docTypeRenameUnsupported");
  }
  return text || t("sidebar.renameFailed");
}

function moveErrorMessage(err: unknown, t: Translate) {
  const { code, text } = backendError(err);
  if (code === "move-into-self") {
    return t("sidebar.moveIntoSelf");
  }
  return text || t("sidebar.moveFailed");
}

/* 目录树内部拖拽是否允许把这个条目放进 destDir：拖回原目录、拖到它自己
   或把它拖进自己的子目录都没有意义。 */
function canMoveInto(source: FileEntry, destDir: string) {
  if (!destDir || destDir === dirName(source.path)) {
    return false;
  }
  return !(source.isDir && isSameOrInside(source.path, destDir));
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
  error,
  onCancel,
  onSubmit,
}: {
  depth: number;
  kind: "folder" | "document";
  error?: string;
  onCancel: () => void;
  onSubmit: (name: string) => void | Promise<void>;
}) {
  const { t } = useSettings();
  const defaultName = kind === "folder" ? t("sidebar.defaultFolder") : t("sidebar.defaultDocument");

  return (
    <div>
      <div
        className="flex items-center gap-1.5 py-1 pr-2"
        style={{ paddingLeft: 8 + depth * 14 }}
      >
        <span className="w-3.5" />
        {kind === "folder" ? (
          <FolderIcon className="h-4 w-4 shrink-0 text-stone-500" />
        ) : (
          <FilePlusIcon className="h-4 w-4 shrink-0 text-stone-500" />
        )}
        <NameInput defaultName={defaultName} selectStem onCancel={onCancel} onSubmit={onSubmit} />
      </div>
      {error && <NameError depth={depth} message={error} />}
    </div>
  );
}

/* 名称输入失败时的提示行，缩进与所在行的文件名对齐。 */
function NameError({ depth, message }: { depth: number; message: string }) {
  return (
    <p
      className="pb-1 pr-2 text-xs leading-5 text-red-600"
      style={{ paddingLeft: 8 + depth * 14 + 20 }}
    >
      {message}
    </p>
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
  flashPaths,
  dragSource,
  treeRevision,
  onOpen,
  onRename,
  onDelete,
  onDragSourceChange,
  onMoveInto,
}: {
  entry: FileEntry;
  depth: number;
  rootPath: string;
  activePath?: string;
  hiddenResourcePath?: string;
  /* 刚被拖拽导入进来的条目路径，由所在的父目录传下来做一次高亮。 */
  flashPaths?: string[];
  /* 当前正在被拖拽的目录树条目，以及每次移动完成后递增的刷新令牌。 */
  dragSource: FileEntry | null;
  treeRevision: number;
  onOpen: (entry: FileEntry) => void;
  onRename: (path: string, name: string) => Promise<FileEntry | null>;
  onDelete: (path: string, mode: DeleteMode) => Promise<boolean>;
  onDragSourceChange: (entry: FileEntry | null) => void;
  onMoveInto: (entry: FileEntry, destDir: string) => Promise<FileEntry | null>;
}) {
  const { t } = useSettings();
  const [open, setOpen] = useState(depth === 0);
  const [children, setChildren] = useState<FileEntry[] | null>(entry.isDir ? null : []);
  const [creating, setCreating] = useState<"folder" | "document" | null>(null);
  const [createError, setCreateError] = useState("");
  const [renaming, setRenaming] = useState(false);
  const [renameError, setRenameError] = useState("");
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [flashedPaths, setFlashedPaths] = useState<string[]>([]);
  const [dropActive, setDropActive] = useState(false);
  const rowRef = useRef<HTMLDivElement>(null);
  const flashTimer = useRef<number | null>(null);
  const revisionRef = useRef(treeRevision);

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
    setCreateError("");
    setRenaming(false);
    setRenameError("");
    setMenu(null);
    setConfirmingDelete(false);
  }, [entry.path, entry.isDir]);

  useEffect(() => {
    if (!entry.isDir || !open || children) {
      return;
    }
    loadChildren().catch(() => setChildren([]));
  }, [entry.isDir, entry.path, open, children]);

  useEffect(() => {
    return () => {
      if (flashTimer.current !== null) {
        window.clearTimeout(flashTimer.current);
      }
    };
  }, []);

  /* 一次拖拽结束后，所有曾经亮起的落点都要复位。 */
  useEffect(() => {
    if (!dragSource) {
      setDropActive(false);
    }
  }, [dragSource]);

  /* 每次移动完成后刷新已展开目录的内容：源目录会少一项，目标目录会多一项。 */
  useEffect(() => {
    if (revisionRef.current === treeRevision) {
      return;
    }
    revisionRef.current = treeRevision;
    if (!entry.isDir || !open) {
      return;
    }
    void loadChildren().catch(() => setChildren([]));
  }, [treeRevision]);

  /* 移动完成后在目标目录里把新条目高亮一下。 */
  useEffect(() => {
    const onMoveDone = (event: Event) => {
      const detail = (event as CustomEvent).detail as TreeMoveResult | null;
      if (!detail || detail.destDir !== entry.path) {
        return;
      }
      setFlashedPaths([detail.to]);
      if (flashTimer.current !== null) {
        window.clearTimeout(flashTimer.current);
      }
      flashTimer.current = window.setTimeout(() => setFlashedPaths([]), 1700);
    };
    window.addEventListener(TREE_MOVE_DONE_EVENT, onMoveDone);
    return () => window.removeEventListener(TREE_MOVE_DONE_EVENT, onMoveDone);
  }, [entry.path]);

  /* 拖拽导入完成后，落点目录（有可能就是这棵树里的某一行）重新列出内容，
     并把新出现的条目短暂高亮一下。 */
  useEffect(() => {
    if (!entry.isDir) {
      return;
    }
    const onDropDone = (event: Event) => {
      const detail = (event as CustomEvent).detail as FileDropResult | null;
      if (!detail || detail.destDir !== entry.path) {
        return;
      }
      const imported = detail.imported ?? [];
      if (imported.length === 0) {
        return;
      }
      void AppService.ReadDir(entry.path)
        .then((rows) => setChildren(sortEntries(rows)))
        .catch(() => undefined);
      setFlashedPaths(imported.map((item) => item.path));
      if (flashTimer.current !== null) {
        window.clearTimeout(flashTimer.current);
      }
      flashTimer.current = window.setTimeout(() => setFlashedPaths([]), 1700);
    };
    window.addEventListener(FILE_DROP_DONE_EVENT, onDropDone);
    return () => window.removeEventListener(FILE_DROP_DONE_EVENT, onDropDone);
  }, [entry.isDir, entry.path]);

  const holdsActive =
    entry.isDir && !!activePath && activePath !== entry.path && isSameOrInside(entry.path, activePath);

  useEffect(() => {
    if (holdsActive) {
      setOpen(true);
    }
  }, [holdsActive]);

  const startCreate = (kind: "folder" | "document") => {
    setCreating(kind);
    setCreateError("");
    setRenaming(false);
    setRenameError("");
    setOpen(true);
    if (children === null) {
      loadChildren().catch(() => setChildren([]));
    }
  };

  const finishCreate = async (kind: "folder" | "document", name: string) => {
    let created: FileEntry | null;
    try {
      created =
        kind === "folder"
          ? await AppService.CreateFolder(entry.path, name)
          : await AppService.CreateDocument(entry.path, name);
    } catch (err) {
      setCreateError(createErrorMessage(err, t));
      throw err;
    }
    setCreating(null);
    setCreateError("");
    await loadChildren();
    if (created && !created.isDir) {
      onOpen(created);
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
    try {
      const next = await handleRename(entry.path, name);
      if (!next) {
        throw new Error(t("sidebar.renameFailed"));
      }
      setRenaming(false);
      setRenameError("");
    } catch (err) {
      setRenameError(renameErrorMessage(err, t));
      throw err;
    }
  };

  const startRename = () => {
    setCreating(null);
    setCreateError("");
    setRenameError("");
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
  /* 文件行也算它所在目录的落点，这样把文件拖到某个文件上就是放进同一个文件夹。 */
  const dropDir = entry.isDir ? entry.path : dirName(entry.path);
  const dropLabel = entry.isDir ? entry.name : baseName(dirName(entry.path));
  const dragSourceIsSelf = dragSource?.path === entry.path;
  const dropAllowed = !!dragSource && !dragSourceIsSelf && canMoveInto(dragSource, dropDir);

  const startDragging = (event: ReactDragEvent<HTMLDivElement>) => {
    if (renaming) {
      event.preventDefault();
      return;
    }
    event.stopPropagation();
    event.dataTransfer.effectAllowed = "move";
    /* 自定义类型把这次拖拽标记成目录树内部操作，避免和系统文件拖拽、
       文本拖拽混在一起。 */
    event.dataTransfer.setData(TREE_DRAG_MIME, entry.path);
    setTreeDragActive(true);
    onDragSourceChange(entry);
  };

  const stopDragging = () => {
    setTreeDragActive(false);
    setDropActive(false);
    onDragSourceChange(null);
  };

  const dragOverRow = (event: ReactDragEvent<HTMLDivElement>) => {
    if (!dragSource) {
      return;
    }
    event.stopPropagation();
    if (!dropAllowed) {
      return;
    }
    event.preventDefault();
    event.dataTransfer.dropEffect = "move";
    if (!dropActive) {
      setDropActive(true);
    }
  };

  const dragLeaveRow = (event: ReactDragEvent<HTMLDivElement>) => {
    if (event.currentTarget.contains(event.relatedTarget as Node | null)) {
      return;
    }
    setDropActive(false);
  };

  const dropOnRow = (event: ReactDragEvent<HTMLDivElement>) => {
    if (!dragSource) {
      return;
    }
    /* 内部拖拽的落点必须自己拦下默认行为，不能落回 WebView 的文本/文件处理。 */
    event.preventDefault();
    event.stopPropagation();
    setTreeDragActive(false);
    setDropActive(false);
    const source = dragSource;
    onDragSourceChange(null);
    if (!dropAllowed) {
      return;
    }
    if (entry.isDir) {
      setOpen(true);
    }
    void onMoveInto(source, dropDir);
  };

  useEffect(() => {
    if (selected) {
      rowRef.current?.scrollIntoView({ block: "nearest" });
    }
  }, [selected]);

  return (
    <div>
      <div
        ref={rowRef}
        data-file-drop-target="tree-row"
        data-drop-dir={dropDir}
        data-drop-label={dropLabel}
        draggable={depth > 0 && !renaming}
        onDragStart={startDragging}
        onDragEnd={stopDragging}
        onDragOver={dragOverRow}
        onDragLeave={dragLeaveRow}
        onDrop={dropOnRow}
        className={cn(
          "group/row relative flex w-full items-center text-[13px] leading-5 select-none",
          selected ? "bg-[var(--vd-selected)] text-[var(--vd-fg)]" : "text-[var(--vd-fg-muted)] hover:bg-[var(--vd-hover)]",
          !selected && menu && "bg-[var(--vd-hover)]",
          flashPaths?.includes(entry.path) && "vd-drop-flash",
          dragSourceIsSelf && "opacity-40",
          dropActive && dropAllowed && "vd-tree-drop-target",
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
                onCancel={() => {
                  setRenaming(false);
                  setRenameError("");
                }}
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
            <ActionIconButton title={t("sidebar.newDocument")} onClick={() => startCreate("document")}>
              <FilePlusIcon className="h-3.5 w-3.5" />
            </ActionIconButton>
          </div>
        )}
      </div>
      {renaming && renameError && <NameError depth={depth} message={renameError} />}
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
              error={createError}
              onCancel={() => {
                setCreating(null);
                setCreateError("");
              }}
              onSubmit={(name) => finishCreate(creating, name)}
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
              flashPaths={flashedPaths}
              dragSource={dragSource}
              treeRevision={treeRevision}
              onOpen={onOpen}
              onRename={handleRename}
              onDelete={handleDeleteChild}
              onDragSourceChange={onDragSourceChange}
              onMoveInto={onMoveInto}
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
  onMove: (path: string, destDir: string) => Promise<FileEntry | null>;
};

const SIDEBAR_DEFAULT_WIDTH = 256;
const SIDEBAR_MIN_WIDTH = 180;
const SIDEBAR_MAX_WIDTH = 480;

function clampSidebarWidth(width: number) {
  const room = typeof window === "undefined" ? SIDEBAR_MAX_WIDTH : window.innerWidth - 360;
  const max = Math.max(SIDEBAR_MIN_WIDTH, Math.min(SIDEBAR_MAX_WIDTH, room));
  return Math.min(max, Math.max(SIDEBAR_MIN_WIDTH, Math.round(width)));
}

export function Sidebar({ rootPath, rootName, activePath, onOpen, onRename, onDelete, onMove }: Props) {
  const { applied, t } = useSettings();
  const [width, setWidth] = useState(SIDEBAR_DEFAULT_WIDTH);
  const [dragging, setDragging] = useState(false);
  const [dragSource, setDragSource] = useState<FileEntry | null>(null);
  const [treeRevision, setTreeRevision] = useState(0);
  const [moveError, setMoveError] = useState("");
  const moveErrorTimer = useRef<number | null>(null);
  const widthRef = useRef(width);
  widthRef.current = width;

  const handleMoveInto = async (source: FileEntry, destDir: string) => {
    try {
      const moved = await onMove(source.path, destDir);
      if (!moved) {
        return null;
      }
      setMoveError("");
      setTreeRevision((value) => value + 1);
      window.dispatchEvent(
        new CustomEvent<TreeMoveResult>(TREE_MOVE_DONE_EVENT, {
          detail: { from: source.path, to: moved.path, isDir: moved.isDir, destDir },
        }),
      );
      return moved;
    } catch (err) {
      setMoveError(moveErrorMessage(err, t));
      if (moveErrorTimer.current !== null) {
        window.clearTimeout(moveErrorTimer.current);
      }
      moveErrorTimer.current = window.setTimeout(() => setMoveError(""), 4200);
      return null;
    }
  };

  useEffect(() => {
    return () => {
      setTreeDragActive(false);
      if (moveErrorTimer.current !== null) {
        window.clearTimeout(moveErrorTimer.current);
      }
    };
  }, []);

  /* 拖到窗口外松手时，React 的 dragend 有时收不到；在窗口层面兜一次。 */
  useEffect(() => {
    const onDragEnd = () => {
      setTreeDragActive(false);
      setDragSource(null);
    };
    window.addEventListener("dragend", onDragEnd, true);
    return () => window.removeEventListener("dragend", onDragEnd, true);
  }, []);

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
          dragSource={dragSource}
          treeRevision={treeRevision}
          onOpen={onOpen}
          onRename={onRename}
          onDelete={onDelete}
          onDragSourceChange={setDragSource}
          onMoveInto={handleMoveInto}
        />
      </div>
      {moveError && (
        <div className="titlebar-no-drag pointer-events-none absolute inset-x-2 bottom-14 z-20 rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg)] px-2 py-1 text-xs leading-5 text-red-600 shadow-md">
          {moveError}
        </div>
      )}
      <div className="titlebar-no-drag flex items-center justify-between gap-2 border-t border-[var(--vd-border)] p-2">
        <SettingsButton />
        <UpdateButton />
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
