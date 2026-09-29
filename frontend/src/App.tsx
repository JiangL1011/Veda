import { useCallback, useEffect, useRef, useState } from "react";
import type { FileEntry, SearchMatch } from "../bindings/veda/models";
import { EditorPane } from "./components/EditorPane";
import { LaunchPage } from "./components/LaunchPage";
import type { DeleteMode } from "./components/ConfirmDeleteModal";
import { SettingsButton, SettingsModal } from "./components/SettingsModal";
import { Sidebar } from "./components/Sidebar";
import { WorkspaceSearchModal } from "./components/WorkspaceSearchModal";
import { SettingsProvider, useSettings } from "./lib/SettingsContext";
import { AppService, asWorkspace, type Workspace } from "./lib/api";
import { EMPTY_WORKSPACE_SEARCH, type RevealTarget, type WorkspaceSearchSession } from "./lib/search";
import { hydrateSettings } from "./lib/settings";
import { isFindFieldTarget, isRecordingShortcut, matchesShortcut } from "./lib/shortcuts";
import { isSameOrInside, launchWorkspace, rewritePath, shouldRestoreOnLaunch } from "./lib/utils";

function fileName(path: string) {
  const parts = path.split(/[/\\]/);
  return parts[parts.length - 1] || path;
}

function AppShell({
  workspace,
  activePath,
  pathRewrite,
  forgetPath,
  revealTarget,
  saveRef,
  closeTabRef,
  openFile,
  openFolder,
  openTreeFile,
  onTabPathChange,
  renameTreeEntry,
  deleteTreeEntry,
  onJumpMatch,
}: {
  workspace: Workspace | null;
  activePath: string | null;
  pathRewrite: { from: string; to: string } | null;
  forgetPath: string | null;
  revealTarget: RevealTarget | null;
  saveRef: React.MutableRefObject<() => Promise<void>>;
  closeTabRef: React.MutableRefObject<() => boolean>;
  openFile: () => Promise<void>;
  openFolder: () => Promise<void>;
  openTreeFile: (entry: FileEntry) => Promise<void>;
  onTabPathChange: (path: string | null) => Promise<void>;
  renameTreeEntry: (path: string, newName: string) => Promise<FileEntry | null>;
  deleteTreeEntry: (path: string, mode: DeleteMode) => Promise<boolean>;
  onJumpMatch: (path: string, match: SearchMatch) => void;
}) {
  const { ready, modalOpen, applied, t } = useSettings();
  const [workspaceSearchOpen, setWorkspaceSearchOpen] = useState(false);
  const [workspaceSearch, setWorkspaceSearch] = useState<WorkspaceSearchSession>(EMPTY_WORKSPACE_SEARCH);
  const showSidebar = workspace?.kind === "directory";
  const workspaceKey = workspace?.kind === "directory" ? workspace.path : "";

  useEffect(() => {
    setWorkspaceSearch(EMPTY_WORKSPACE_SEARCH);
  }, [workspaceKey]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (isRecordingShortcut() || modalOpen) {
        return;
      }
      if (matchesShortcut(event, applied.shortcuts.searchWorkspace)) {
        event.preventDefault();
        event.stopPropagation();
        setWorkspaceSearchOpen(true);
        return;
      }
      if (!workspaceSearchOpen && matchesShortcut(event, applied.shortcuts.closeTab)) {
        event.preventDefault();
        event.stopPropagation();
        /* 最后一个标签页关掉之后，再按一次就关闭窗口。 */
        if (!closeTabRef.current()) {
          void AppService.CloseWindow();
        }
        return;
      }
      if (matchesShortcut(event, applied.shortcuts.searchCurrentFile)) {
        if (isFindFieldTarget(event.target) && (event.target as HTMLElement).closest("[data-vd-find=workspace]")) {
          return;
        }
        event.preventDefault();
        event.stopPropagation();
        window.dispatchEvent(new Event("veda:search-current"));
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [applied.shortcuts, closeTabRef, modalOpen, workspaceSearchOpen]);

  if (!ready) {
    return <div className="h-full bg-[var(--vd-bg-subtle)]" />;
  }

  const searchModal = workspaceSearchOpen && (
    <WorkspaceSearchModal
      workspacePath={workspace?.kind === "directory" ? workspace.path : null}
      t={t}
      session={workspaceSearch}
      onSessionChange={setWorkspaceSearch}
      onClose={() => setWorkspaceSearchOpen(false)}
      onJump={({ path, match }) => onJumpMatch(path, match)}
    />
  );

  if (!workspace) {
    return (
      <>
        <LaunchPage onOpenFile={() => void openFile()} onOpenFolder={() => void openFolder()} />
        {modalOpen && <SettingsModal />}
        {searchModal}
      </>
    );
  }

  return (
    <div className="relative flex h-full bg-[var(--vd-bg)]">
      {showSidebar && (
        <Sidebar
          rootPath={workspace.path}
          rootName={fileName(workspace.path)}
          activePath={activePath ?? undefined}
          onOpen={(entry) => void openTreeFile(entry)}
          onRename={renameTreeEntry}
          onDelete={deleteTreeEntry}
        />
      )}
      <EditorPane
        key={workspace.path}
        path={activePath}
        showSidebar={showSidebar}
        workspacePath={showSidebar ? workspace.path : ""}
        pathRewrite={pathRewrite}
        forgetPath={forgetPath}
        revealTarget={revealTarget}
        onPathChange={(path) => void onTabPathChange(path)}
        onDirtySaveRef={(fn) => {
          saveRef.current = fn;
        }}
        onCloseTabRef={(fn) => {
          closeTabRef.current = fn;
        }}
      />
      {!showSidebar && (
        <div className="titlebar-no-drag pointer-events-none absolute bottom-3 left-3 z-20">
          <div className="pointer-events-auto rounded-md border border-[var(--vd-border)] bg-[var(--vd-bg-muted)] p-1 shadow-sm">
            <SettingsButton />
          </div>
        </div>
      )}
      {modalOpen && <SettingsModal />}
      {searchModal}
    </div>
  );
}

export default function App() {
  const [ready, setReady] = useState(false);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [activePath, setActivePath] = useState<string | null>(null);
  const [pathRewrite, setPathRewrite] = useState<{ from: string; to: string } | null>(null);
  const [forgetPath, setForgetPath] = useState<string | null>(null);
  const [revealTarget, setRevealTarget] = useState<RevealTarget | null>(null);
  const saveRef = useRef<() => Promise<void>>(async () => undefined);
  const closeTabRef = useRef<() => boolean>(() => false);

  const applyWorkspace = useCallback(async (next: Workspace) => {
    setWorkspace(next);
    if (next.kind === "file") {
      setActivePath(next.path);
      await AppService.SaveSession("file", next.path, "");
      return;
    }
    const file = next.file || null;
    setActivePath(file);
    await AppService.SaveSession("directory", next.path, file || "");
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const spawned = asWorkspace(launchWorkspace());
      if (spawned) {
        await applyWorkspace(spawned);
        setReady(true);
        return;
      }
      if (!shouldRestoreOnLaunch()) {
        setReady(true);
        return;
      }
      try {
        const bundle = await AppService.GetSettings("").catch(() => null);
        const rec = (bundle ?? {}) as Record<string, unknown>;
        const globalSettings = hydrateSettings(rec.global ?? rec.Global);
        if (globalSettings.general.startup === "launch") {
          return;
        }
        const session = await AppService.GetSession();
        const raw = session as unknown as { Last?: unknown; last?: unknown } | null;
        const last = asWorkspace(raw?.Last ?? raw?.last);
        if (!cancelled && last) {
          await applyWorkspace(last);
        }
      } finally {
        if (!cancelled) {
          setReady(true);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [applyWorkspace]);

  useEffect(() => {
    const onSave = () => {
      void saveRef.current();
    };
    window.addEventListener("veda:save", onSave);
    return () => {
      window.removeEventListener("veda:save", onSave);
    };
  }, []);

  const openFile = async () => {
    try {
      const target = await AppService.PickFile();
      const ws = asWorkspace(target);
      if (ws) {
        await applyWorkspace(ws);
      }
    } catch {
      /* 用户取消 */
    }
  };

  const openFolder = async () => {
    try {
      const target = await AppService.PickDirectory();
      const ws = asWorkspace(target);
      if (ws) {
        await applyWorkspace(ws);
      }
    } catch {
      /* 用户取消 */
    }
  };

  const openTreeFile = async (entry: FileEntry) => {
    if (!workspace || workspace.kind !== "directory") {
      return;
    }
    setActivePath(entry.path);
    await AppService.SaveSession("directory", workspace.path, entry.path);
  };

  const changeTabPath = async (path: string | null) => {
    if (!workspace) {
      return;
    }
    setActivePath(path);
    if (workspace.kind === "directory") {
      await AppService.SaveSession("directory", workspace.path, path ?? "");
    }
  };

  const jumpToMatch = async (path: string, match: SearchMatch) => {
    if (workspace?.kind === "directory") {
      setActivePath(path);
      await AppService.SaveSession("directory", workspace.path, path);
    } else if (workspace?.kind === "file") {
      setActivePath(workspace.path);
    }
    setRevealTarget({ path, match, nonce: Date.now() });
  };

  const renameTreeEntry = async (path: string, newName: string) => {
    if (!workspace) {
      return null;
    }
    if (activePath && isSameOrInside(path, activePath)) {
      await saveRef.current();
    }
    const next = await AppService.Rename(path, newName);
    if (!next) {
      return null;
    }
    setPathRewrite({ from: path, to: next.path });
    const newRoot = rewritePath(path, next.path, workspace.path);
    const newFile = activePath ? rewritePath(path, next.path, activePath) : "";
    if (newRoot !== workspace.path || newFile !== (activePath ?? "")) {
      await applyWorkspace({
        kind: workspace.kind,
        path: newRoot,
        file: workspace.kind === "directory" ? newFile || undefined : undefined,
      });
    }
    return next;
  };

  const deleteTreeEntry = async (path: string, mode: DeleteMode) => {
    if (!workspace || workspace.kind !== "directory") {
      return false;
    }
    if (path === workspace.path) {
      return false;
    }
    if (activePath && isSameOrInside(path, activePath)) {
      await saveRef.current();
    }
    if (mode === "trash") {
      await AppService.MoveToTrash(path);
    } else {
      await AppService.Delete(path);
    }
    setForgetPath(path);
    if (activePath && isSameOrInside(path, activePath)) {
      setActivePath(null);
      await AppService.SaveSession("directory", workspace.path, "");
    }
    return true;
  };

  if (!ready) {
    return <div className="h-full bg-[var(--vd-bg-subtle)]" />;
  }

  const canUseWorkspace = workspace?.kind === "directory";

  return (
    <SettingsProvider workspacePath={canUseWorkspace ? workspace.path : ""} canUseWorkspace={!!canUseWorkspace}>
      <AppShell
        workspace={workspace}
        activePath={activePath}
        pathRewrite={pathRewrite}
        forgetPath={forgetPath}
        revealTarget={revealTarget}
        saveRef={saveRef}
        closeTabRef={closeTabRef}
        openFile={openFile}
        openFolder={openFolder}
        openTreeFile={openTreeFile}
        onTabPathChange={changeTabPath}
        renameTreeEntry={renameTreeEntry}
        deleteTreeEntry={deleteTreeEntry}
        onJumpMatch={(path, match) => void jumpToMatch(path, match)}
      />
    </SettingsProvider>
  );
}
