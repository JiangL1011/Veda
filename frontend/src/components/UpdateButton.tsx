import { useSettings } from "../lib/SettingsContext";
import { useUpdate } from "../lib/UpdateContext";
import { cn } from "../lib/utils";
import { DownloadIcon } from "./Icons";

/**
 * 新版本下载完成后，在左下角的设置按钮旁给出一个文字入口。
 * 点击后直接打开设置并定位到「关于」，由用户在那里完成重启安装。
 */
export function UpdateButton({ className }: { className?: string }) {
  const { t, openModal } = useSettings();
  const { ready } = useUpdate();

  if (!ready) {
    return null;
  }

  return (
    <button
      type="button"
      title={t("update.buttonHint")}
      className={cn(
        "inline-flex h-8 min-w-0 cursor-pointer items-center gap-1.5 rounded-md border border-[var(--vd-update-border)] bg-[var(--vd-update-bg)] px-2 text-xs font-medium text-[var(--vd-update-fg)] transition-colors hover:bg-[var(--vd-update-hover)]",
        className,
      )}
      onClick={() => openModal("about")}
    >
      <DownloadIcon className="h-4 w-4 shrink-0" />
      <span className="min-w-0 truncate">{t("update.button")}</span>
    </button>
  );
}
