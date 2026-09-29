import { forwardRef, useImperativeHandle, useRef } from "react";
import { utf16FromUtf8Offset } from "../lib/search";
import { cn } from "../lib/utils";

type Props = {
  value: string;
  editable: boolean;
  onChange: (next: string) => void;
};

export type TextViewerHandle = {
  revealBytes: (startByte: number, endByte: number) => void;
};

export const TextViewer = forwardRef<TextViewerHandle, Props>(function TextViewer(
  { value, editable, onChange },
  ref,
) {
  const areaRef = useRef<HTMLTextAreaElement | null>(null);

  useImperativeHandle(ref, () => ({
    revealBytes(startByte, endByte) {
      const el = areaRef.current;
      if (!el) {
        return;
      }
      const start = utf16FromUtf8Offset(value, startByte);
      const end = utf16FromUtf8Offset(value, endByte);
      el.focus();
      el.setSelectionRange(start, Math.max(start, end));
      const lineHeight = 24;
      const before = value.slice(0, start);
      const line = before.split("\n").length - 1;
      el.scrollTop = Math.max(0, line * lineHeight - el.clientHeight / 3);
    },
  }), [value]);

  return (
    <textarea
      ref={areaRef}
      className={cn(
        "titlebar-no-drag h-full w-full resize-none bg-[var(--vd-bg-subtle)] px-8 py-6 font-mono text-[13px] leading-6 text-[var(--vd-fg)] outline-none",
      )}
      value={value}
      readOnly={!editable}
      spellCheck={false}
      onChange={(e) => onChange(e.target.value)}
    />
  );
});
