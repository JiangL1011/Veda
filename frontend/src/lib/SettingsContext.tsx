import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import type { SettingsBundle } from "../../bindings/veda/models";
import { AppService } from "./api";
import { translate } from "./i18n";
import {
  cloneSettings,
  defaultSettings,
  hydrateSettings,
  markdownStyleVars,
  normalizeLanguage,
  resolveTheme,
  type AppSettings,
  type Language,
  type SettingsScope,
} from "./settings";

type Ctx = {
  ready: boolean;
  scope: SettingsScope;
  setScope: (scope: SettingsScope) => void;
  canUseWorkspace: boolean;
  workspacePath: string;
  current: AppSettings;
  globalSettings: AppSettings;
  applied: AppSettings;
  updateCurrent: (next: AppSettings) => void;
  resolvedTheme: "light" | "dark";
  language: Language;
  t: (key: string) => string;
  modalOpen: boolean;
  openModal: () => void;
  closeModal: () => void;
};

const SettingsContext = createContext<Ctx | null>(null);

function applyDocumentTheme(settings: AppSettings, theme: "light" | "dark") {
  const root = document.documentElement;
  root.dataset.theme = theme;
  root.lang = normalizeLanguage(settings.general.language);
  const vars = markdownStyleVars(settings.markdown.styles, theme);
  for (const [key, value] of Object.entries(vars)) {
    root.style.setProperty(key, value);
  }
}

function readBundle(bundle: SettingsBundle | Record<string, unknown> | null) {
  const rec = (bundle ?? {}) as Record<string, unknown>;
  const globalSettings = hydrateSettings(rec.global ?? rec.Global);
  const exists = Boolean(rec.workspaceExists ?? rec.WorkspaceExists);
  const workspaceSettings = exists
    ? hydrateSettings(rec.workspace ?? rec.Workspace)
    : cloneSettings(globalSettings);
  workspaceSettings.general.startup = globalSettings.general.startup;
  return { globalSettings, workspaceSettings, exists };
}

export function SettingsProvider({
  workspacePath,
  canUseWorkspace,
  children,
}: {
  workspacePath: string;
  canUseWorkspace: boolean;
  children: ReactNode;
}) {
  const [ready, setReady] = useState(false);
  const [scope, setScopeState] = useState<SettingsScope>("global");
  const [global, setGlobal] = useState<AppSettings>(defaultSettings);
  const [workspace, setWorkspace] = useState<AppSettings>(defaultSettings);
  const [workspaceExists, setWorkspaceExists] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [systemDark, setSystemDark] = useState(() =>
    typeof window !== "undefined" && window.matchMedia
      ? window.matchMedia("(prefers-color-scheme: dark)").matches
      : false,
  );
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pending = useRef<{ scope: SettingsScope; settings: AppSettings } | null>(null);
  const workspaceExistsRef = useRef(false);
  workspaceExistsRef.current = workspaceExists;

  const applyBundle = useCallback((bundle: SettingsBundle | Record<string, unknown> | null) => {
    const next = readBundle(bundle);
    setGlobal(next.globalSettings);
    setWorkspace(next.workspaceSettings);
    setWorkspaceExists(next.exists);
  }, []);

  const persist = useCallback(
    (nextScope: SettingsScope, settings: AppSettings) => {
      pending.current = { scope: nextScope, settings };
      if (saveTimer.current) {
        window.clearTimeout(saveTimer.current);
      }
      saveTimer.current = window.setTimeout(() => {
        const job = pending.current;
        pending.current = null;
        if (!job) {
          return;
        }
        const path = job.scope === "workspace" ? workspacePath : "";
        void AppService.SaveSettings(job.scope, path, JSON.stringify(job.settings))
          .then((bundle) => {
            if (bundle) {
              applyBundle(bundle);
            }
          })
          .catch((err) => {
            console.error("save settings failed", err);
          });
      }, 280);
    },
    [workspacePath, applyBundle],
  );

  useEffect(() => {
    return () => {
      if (saveTimer.current) {
        window.clearTimeout(saveTimer.current);
      }
      const job = pending.current;
      pending.current = null;
      if (!job) {
        return;
      }
      const path = job.scope === "workspace" ? workspacePath : "";
      void AppService.SaveSettings(job.scope, path, JSON.stringify(job.settings)).catch((err) => {
        console.error("save settings failed", err);
      });
    };
  }, [workspacePath]);

  useEffect(() => {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => setSystemDark(media.matches);
    onChange();
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const path = canUseWorkspace ? workspacePath : "";
    AppService.GetSettings(path)
      .then((bundle) => {
        if (cancelled || !bundle) {
          return;
        }
        applyBundle(bundle);
        if (!canUseWorkspace) {
          setScopeState("global");
        }
      })
      .catch(() => {
        if (!cancelled) {
          const fallback = defaultSettings();
          setGlobal(fallback);
          setWorkspace(cloneSettings(fallback));
          setWorkspaceExists(false);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setReady(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [workspacePath, canUseWorkspace, applyBundle]);

  const setScope = useCallback(
    (next: SettingsScope) => {
      if (next === "workspace" && !canUseWorkspace) {
        return;
      }
      setScopeState(next);
    },
    [canUseWorkspace],
  );

  const current = scope === "workspace" && canUseWorkspace ? workspace : global;
  const applied = canUseWorkspace ? workspace : global;

  const resolvedTheme = resolveTheme(applied.general.theme, systemDark);
  const language = normalizeLanguage(applied.general.language);

  useLayoutEffect(() => {
    applyDocumentTheme(applied, resolvedTheme);
  }, [applied, resolvedTheme]);

  const updateCurrent = useCallback(
    (next: AppSettings) => {
      if (scope === "workspace" && canUseWorkspace) {
        const pinned = {
          ...next,
          general: { ...next.general, startup: global.general.startup },
        };
        setWorkspace(pinned);
        persist("workspace", pinned);
        return;
      }
      setGlobal(next);
      if (!workspaceExistsRef.current) {
        setWorkspace(cloneSettings(next));
      }
      persist("global", next);
    },
    [scope, canUseWorkspace, persist, global.general.startup],
  );

  const t = useCallback((key: string) => translate(language, key), [language]);

  const value = useMemo<Ctx>(
    () => ({
      ready,
      scope: canUseWorkspace ? scope : "global",
      setScope,
      canUseWorkspace,
      workspacePath,
      current,
      globalSettings: global,
      applied,
      updateCurrent,
      resolvedTheme,
      language,
      t,
      modalOpen,
      openModal: () => setModalOpen(true),
      closeModal: () => setModalOpen(false),
    }),
    [
      ready,
      scope,
      setScope,
      canUseWorkspace,
      workspacePath,
      current,
      global,
      applied,
      updateCurrent,
      resolvedTheme,
      language,
      t,
      modalOpen,
    ],
  );

  return <SettingsContext.Provider value={value}>{children}</SettingsContext.Provider>;
}

export function useSettings() {
  const ctx = useContext(SettingsContext);
  if (!ctx) {
    throw new Error("useSettings must be used within SettingsProvider");
  }
  return ctx;
}
