import { useSettings } from "../lib/SettingsContext";
import { FileIcon, FolderIcon } from "./Icons";
import { SettingsButton } from "./SettingsModal";

type Props = {
  onOpenFile: () => void;
  onOpenFolder: () => void;
};

export function LaunchPage({ onOpenFile, onOpenFolder }: Props) {
  const { t } = useSettings();
  return (
    <div className="relative flex min-h-full flex-col items-center justify-center bg-[var(--vd-bg-subtle)] px-8">
      <div className="titlebar-drag absolute inset-x-0 top-0 h-[var(--vd-titlebar-height)]" />
      <div className="titlebar-no-drag relative z-10 w-full max-w-xl text-center">
        <p className="text-xs font-semibold tracking-[0.22em] text-[var(--vd-fg-subtle)]">{t("app.brand")}</p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight text-[var(--vd-fg)]">{t("launch.title")}</h1>
        <p className="mt-2 text-sm text-[var(--vd-fg-muted)]">{t("launch.subtitle")}</p>
        <div className="mt-10 grid grid-cols-2 gap-4">
          <button
            type="button"
            onClick={onOpenFile}
            className="group rounded-2xl border border-[var(--vd-border)] bg-[var(--vd-bg)] p-8 text-left shadow-sm transition hover:-translate-y-0.5 hover:border-[var(--vd-fg-subtle)] hover:shadow-md"
          >
            <FileIcon className="h-8 w-8 text-[var(--vd-fg)]" />
            <div className="mt-5 text-lg font-medium text-[var(--vd-fg)]">{t("launch.openFile")}</div>
            <div className="mt-1 text-sm text-[var(--vd-fg-muted)]">{t("launch.openFileHint")}</div>
          </button>
          <button
            type="button"
            onClick={onOpenFolder}
            className="group rounded-2xl border border-[var(--vd-border)] bg-[var(--vd-bg)] p-8 text-left shadow-sm transition hover:-translate-y-0.5 hover:border-[var(--vd-fg-subtle)] hover:shadow-md"
          >
            <FolderIcon className="h-8 w-8 text-[var(--vd-fg)]" />
            <div className="mt-5 text-lg font-medium text-[var(--vd-fg)]">{t("launch.openFolder")}</div>
            <div className="mt-1 text-sm text-[var(--vd-fg-muted)]">{t("launch.openFolderHint")}</div>
          </button>
        </div>
      </div>
      <div className="titlebar-no-drag absolute bottom-3 left-3 z-20">
        <SettingsButton />
      </div>
    </div>
  );
}
