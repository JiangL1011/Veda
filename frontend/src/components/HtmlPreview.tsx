import { useEffect, useState } from "react";
import { AppService } from "../lib/api";
import { useSettings } from "../lib/SettingsContext";

type Props = {
  path: string;
  workspacePath: string;
};

/* HTML 预览直接把磁盘上的文件交给一个 iframe 渲染，因此页面里的相对引用
   （样式、脚本、图片）会被解析成同一个来源下的兄弟文件，由 PreviewURL 提供的
   路由负责读取。 */
export function HtmlPreview({ path, workspacePath }: Props) {
  const { t } = useSettings();
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setError("");
    setUrl("");
    AppService.PreviewURL(workspacePath, path)
      .then((next) => {
        if (!cancelled) {
          setUrl(next);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setError("failed");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [path, workspacePath]);

  if (error) {
    return <Empty message={t("editor.previewFailed")} />;
  }
  if (!url) {
    return <Empty message={t("editor.loading")} />;
  }

  return (
    <iframe
      title={t("editor.previewHtml")}
      src={url}
      /* 预览的是磁盘上的任意 HTML，所以放进沙箱：允许脚本运行，但不给它应用
         自身的源，页面就无法读写编辑器的状态或调用后端接口。 */
      sandbox="allow-scripts allow-forms allow-modals allow-popups allow-popups-to-escape-sandbox allow-downloads"
      className="h-full w-full border-0 bg-white"
    />
  );
}

function Empty({ message }: { message: string }) {
  return <div className="flex h-full items-center justify-center text-sm text-[var(--vd-fg-muted)]">{message}</div>;
}
