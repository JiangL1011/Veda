import { useEffect, useRef, type CSSProperties } from "react";
import { useSettings } from "../lib/SettingsContext";
import { cn } from "../lib/utils";
import { vditorPreviewOptions } from "../lib/vditor";

type Props = {
  markdown: string;
  className?: string;
  style?: CSSProperties;
};

export function MarkdownPreview({ markdown, className, style }: Props) {
  const { resolvedTheme } = useSettings();
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    let cancelled = false;
    void import("vditor").then(({ default: Vditor }) => {
      if (cancelled || !ref.current) {
        return;
      }
      void Vditor.preview(el, markdown, vditorPreviewOptions(resolvedTheme));
    });
    return () => {
      cancelled = true;
    };
  }, [markdown, resolvedTheme]);

  return <div ref={ref} className={cn("md-wysiwyg md-preview", className)} style={style} />;
}
