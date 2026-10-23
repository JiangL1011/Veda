import { useEffect, useRef, useState } from "react";
import type { DataMigrationState } from "../../bindings/veda/models";
import { AppService } from "../lib/api";
import { translate } from "../lib/i18n";
import { fillTemplate } from "../lib/search";
import { normalizeLanguage, type Language } from "../lib/settings";

// 迁移通常在几毫秒内完成。延迟显示可以避免索引已存在时闪一下遮罩。
const REVEAL_DELAY_MS = 120;

function initialLanguage(): Language {
  if (typeof navigator === "undefined") {
    return "zh-CN";
  }
  return navigator.language.toLowerCase().startsWith("en") ? "en" : "zh-CN";
}

/**
 * 启动迁移期间的全屏遮罩：应用内容在迁移完成前完全不挂载，
 * 因此用户在这个阶段无法进行任何文件操作。
 */
export function DataMigrationOverlay({
  state,
  onRetry,
}: {
  state: DataMigrationState | null;
  onRetry: () => void;
}) {
  const [visible, setVisible] = useState(false);
  const [language, setLanguage] = useState<Language>(initialLanguage);
  const retried = useRef(false);
  const failed = Boolean(state?.error);
  const running = !state || state.running;

  useEffect(() => {
    const timer = window.setTimeout(() => setVisible(true), REVEAL_DELAY_MS);
    return () => window.clearTimeout(timer);
  }, []);

  useEffect(() => {
    let cancelled = false;
    void AppService.GetSettings("")
      .then((bundle) => {
        if (!cancelled && bundle?.global?.general?.language) {
          setLanguage(normalizeLanguage(bundle.global.general.language));
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, []);

  const t = (key: string) => translate(language, key);
  const total = state?.total ?? 0;
  const processed = Math.min(state?.processed ?? 0, total || Number.MAX_SAFE_INTEGER);
  const percent = total > 0 ? Math.min(100, Math.round((processed / total) * 100)) : null;
  const shown = visible || failed;

  return (
    <div
      className="titlebar-no-drag fixed inset-0 z-[200] flex select-none items-center justify-center bg-[var(--vd-bg-subtle)] px-6"
      role="alertdialog"
      aria-busy={running && !failed}
      aria-live="polite"
    >
      <div
        className={`w-[min(420px,92vw)] text-center transition-all duration-200 ${
          shown ? "translate-y-0 opacity-100" : "translate-y-1 opacity-0"
        }`}
      >
        <div className="relative mx-auto mb-7 flex h-16 w-16 items-center justify-center">
          {!failed && (
            <span className="absolute inset-0 animate-spin rounded-full border-2 border-[var(--vd-border)] border-t-[var(--vd-fg)]" />
          )}
          <span
            className={`flex h-9 w-9 items-center justify-center rounded-xl bg-[var(--vd-bg-muted)] text-[11px] font-semibold tracking-[0.18em] text-[var(--vd-fg-muted)] ${
              failed ? "" : "animate-pulse"
            }`}
          >
            VEDA
          </span>
        </div>

        <h1 className="m-0 text-[17px] font-semibold tracking-tight text-[var(--vd-fg)]">
          {failed ? t("migration.failed") : t("migration.title")}
        </h1>
        <p className="mt-2 text-[13px] leading-6 text-[var(--vd-fg-muted)]">
          {failed ? state?.error || t("migration.failedHint") : t("migration.subtitle")}
        </p>

        {!failed && (
          <div className="mx-auto mt-6 h-1 w-full overflow-hidden rounded-full bg-[var(--vd-border)]">
            {percent === null ? (
              <span className="vd-migration-bar block h-full w-1/3 rounded-full bg-[var(--vd-fg-muted)]" />
            ) : (
              <span
                className="block h-full rounded-full bg-[var(--vd-fg-muted)] transition-[width] duration-300 ease-out"
                style={{ width: `${percent}%` }}
              />
            )}
          </div>
        )}

        {!failed && total > 0 && (
          <p className="mt-2.5 text-[11px] tabular-nums text-[var(--vd-fg-subtle)]">
            {fillTemplate(t("migration.progress"), { done: processed, total })}
          </p>
        )}

        {failed ? (
          <button
            type="button"
            className="mt-6 rounded-lg border border-[var(--vd-border)] bg-[var(--vd-bg)] px-5 py-2 text-[13px] font-medium text-[var(--vd-fg)] hover:bg-[var(--vd-hover)]"
            onClick={() => {
              if (retried.current) {
                return;
              }
              retried.current = true;
              onRetry();
              window.setTimeout(() => {
                retried.current = false;
              }, 400);
            }}
          >
            {t("migration.retry")}
          </button>
        ) : (
          <p className="mt-4 text-[11px] text-[var(--vd-fg-subtle)]">{t("migration.blocked")}</p>
        )}
      </div>
    </div>
  );
}
