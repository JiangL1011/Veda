import { useEffect, useState } from "react";
import { AppService } from "../lib/api";
import { useSettings } from "../lib/SettingsContext";

type Props = {
  path: string;
  kind: string;
  name: string;
};

export function MediaViewer({ path, kind, name }: Props) {
  const { t } = useSettings();
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setError("");
    AppService.MediaURL(path)
      .then((u) => {
        if (!cancelled) {
          setUrl(u);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setError(String(err));
        }
      });
    return () => {
      cancelled = true;
    };
  }, [path]);

  if (error) {
    return <Empty message={error} />;
  }
  if (!url) {
    return <Empty message={t("editor.loading")} />;
  }

  if (kind === "video") {
    return (
      <div className="flex h-full items-center justify-center bg-[var(--vd-bg-subtle)] p-6">
        <video className="max-h-full max-w-full" src={url} controls autoPlay={false} />
      </div>
    );
  }

  return (
    <div className="flex h-full items-center justify-center bg-[var(--vd-bg-muted)] p-8">
      <img src={url} alt={name} className="max-h-full max-w-full object-contain shadow-sm" />
    </div>
  );
}

function Empty({ message }: { message: string }) {
  return <div className="flex h-full items-center justify-center text-sm text-[var(--vd-fg-muted)]">{message}</div>;
}
