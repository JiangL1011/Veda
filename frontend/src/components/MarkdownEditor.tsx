import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import type Vditor from "vditor";
import { AppService } from "../lib/api";
import type { SearchMatch } from "../../bindings/veda/models";
import { utf16FromUtf8Offset } from "../lib/search";
import { useSettings } from "../lib/SettingsContext";
import { editorWidthCSS } from "../lib/settings";
import { cn } from "../lib/utils";
import { CloseIcon } from "./Icons";
import { RemoteImageModal } from "./RemoteImageModal";
import {
  markdownFromVditor,
  applyImageMetadata,
  formatImageMetadata,
  imageMarkdownSource,
  parseImageMetadata,
  pinEditorScroll,
  rewriteLocalImages,
  setImageMarkdownSource,
  tagWysiwygPopover,
  VDITOR_CDN,
  VDITOR_CONTENT_THEME,
  VDITOR_CONTENT_THEME_PATH,
  VDITOR_DISABLE_WIDTH_PADDING,
  vditorCodeTheme,
  vditorLang,
  vditorTheme,
  vditorToolbar,
} from "../lib/vditor";

type Props = {
  path: string;
  dir: string;
  value: string;
  editable: boolean;
  onChange: (markdown: string) => void;
  onHeadingsChange?: (headings: MarkdownHeading[]) => void;
};

export type MarkdownHeading = {
  level: number;
  text: string;
};

export type MarkdownEditorHandle = {
  scrollToHeading: (index: number) => void;
  revealMatch: (source: string, match: SearchMatch) => void;
};

function fileAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error);
    reader.onload = () => resolve(String(reader.result || "").split(",").pop() || "");
    reader.readAsDataURL(file);
  });
}

/* 绝对路径在源码里保持可读，只转义那些会提前结束链接地址的字符。 */
function markdownImagePath(path: string) {
  return path.replace(/[%\s()<>"']/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0")}`);
}

function markdownImageLabel(name: string) {
  return name.replace(/\.[^.]+$/, "").replace(/[[\]\\]/g, "\\$&") || "图片";
}

/* URL 自身已经带了百分号转义，所以这里只转义会提前结束链接地址的字符。 */
function markdownRemotePath(rawURL: string) {
  return rawURL.replace(/[\s()<>]/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0")}`);
}

/* 远程图片只做链接引用，所以 URL 原样保留，只取路径最后一段作为图片标题。 */
function remoteImageLabel(rawURL: string) {
  try {
    return markdownImageLabel(decodeURIComponent(new URL(rawURL).pathname.split("/").pop() || ""));
  } catch {
    return markdownImageLabel("");
  }
}

const IMAGE_FORMAT_RANK = ["image/png", "image/jpeg", "image/webp", "image/gif", "image/svg+xml"];

function imageFiles(transfer: DataTransfer | null) {
  return Array.from(transfer?.files ?? []).filter((file) => file.type.startsWith("image/"));
}

/* 剪贴板会同时给出同一张图片的多种格式（WebKit 会在 PNG 旁边再放一份 TIFF），
   它们共用同一个主文件名。文件名不同就说明是不同的图片，因此只有同名的项
   才会合并成其中最优的那种格式。 */
function collapseDuplicateFormats(files: File[]) {
  const names = new Set(files.map((file) => file.name.replace(/\.[^./\\]+$/, "").trim().toLowerCase()));
  if (files.length < 2 || names.size > 1) {
    return files;
  }
  const rank = (file: File) => {
    const at = IMAGE_FORMAT_RANK.indexOf(file.type.toLowerCase());
    return at < 0 ? IMAGE_FORMAT_RANK.length : at;
  };
  return [files.reduce((best, file) => (rank(file) < rank(best) ? file : best))];
}

function linkHref(el: Element): string {
  return (el.getAttribute("href") || el.getAttribute("data-href") || "").trim();
}

function safeDestroy(vditor: Vditor | null) {
  if (!vditor) {
    return;
  }
  try {
    vditor.destroy();
  } catch {
    /* 构造函数可能还在等待 i18n/lute 加载完成 */
  }
}

function isTaskCheckbox(target: EventTarget | null): target is HTMLInputElement {
  return target instanceof HTMLInputElement && target.type === "checkbox";
}

function setTaskCheckboxesLocked(root: ParentNode | null, locked: boolean) {
  if (!root) {
    return;
  }
  root.querySelectorAll<HTMLInputElement>('input[type="checkbox"]').forEach((el) => {
    el.tabIndex = locked ? -1 : 0;
  });
}

export const MarkdownEditor = forwardRef<MarkdownEditorHandle, Props>(function MarkdownEditor(
  { path, dir, value, editable, onChange, onHeadingsChange },
  ref,
) {
  const { applied, language, resolvedTheme, t, workspacePath } = useSettings();
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const hostRef = useRef<HTMLDivElement | null>(null);
  const vdRef = useRef<Vditor | null>(null);
  const readyRef = useRef(false);
  const silentRef = useRef(true);
  const markdownRef = useRef(value);
  const pathRef = useRef(path);
  const dirRef = useRef(dir);
  const editableRef = useRef(editable);
  const onChangeRef = useRef(onChange);
  const onHeadingsChangeRef = useRef(onHeadingsChange);
  const themeRef = useRef(resolvedTheme);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [remoteOpen, setRemoteOpen] = useState(false);
  const [importError, setImportError] = useState("");
  const workspacePathRef = useRef(workspacePath);

  if (pathRef.current !== path) {
    pathRef.current = path;
    markdownRef.current = value;
  }
  dirRef.current = dir;
  editableRef.current = editable;
  onChangeRef.current = onChange;
  onHeadingsChangeRef.current = onHeadingsChange;
  themeRef.current = resolvedTheme;
  workspacePathRef.current = workspacePath;

  const editorWidth = editorWidthCSS(applied.markdown.editorWidth);

  const insertImportedImage = async (file: File) => {
    const vditor = vdRef.current;
    if (!vditor || !editableRef.current) {
      return;
    }
    try {
      const encoded = await fileAsBase64(file);
      const asset = await AppService.ImportImageData(
        workspacePathRef.current,
        file.name || "pasted-image",
        file.type,
        encoded,
      );
      if (!asset) {
        return;
      }
      vditor.insertMD(`![${markdownImageLabel(asset.name)}](${markdownImagePath(asset.markdownPath)})`);
      emit(vditor);
    } catch (err) {
      console.error("import image failed", err);
      setImportError(String(err));
    }
  };

  const chooseLocalImage = () => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = "image/*,.svg";
    input.multiple = true;
    input.addEventListener("change", () => {
      void (async () => {
        for (const file of Array.from(input.files ?? [])) {
          await insertImportedImage(file);
        }
      })();
    });
    input.click();
  };

  const askForRemoteImage = () => {
    setImportError("");
    setRemoteOpen(true);
  };

  /* 网络图片只做链接引用，不会复制到资源目录，所以文档里保留原始地址，
     在渲染时再去加载。 */
  const insertRemoteImage = (rawURL: string) => {
    const vditor = vdRef.current;
    if (!vditor || !editableRef.current) {
      return;
    }
    setRemoteOpen(false);
    setImportError("");
    vditor.insertMD(`![${remoteImageLabel(rawURL)}](${markdownRemotePath(rawURL)})`);
    emit(vditor);
  };

  /* 工具栏在每个 Vditor 实例上只构建一次，所以按钮通过 ref 间接调用，
     而不是捕获本次渲染时的处理函数。 */
  const imageActionsRef = useRef({ chooseLocalImage, askForRemoteImage });
  imageActionsRef.current = { chooseLocalImage, askForRemoteImage };

  useImperativeHandle(ref, () => ({
    scrollToHeading(index) {
      const heading = wrapRef.current?.querySelectorAll<HTMLElement>(
        ".vditor-wysiwyg h1, .vditor-wysiwyg h2, .vditor-wysiwyg h3, .vditor-wysiwyg h4, .vditor-wysiwyg h5, .vditor-wysiwyg h6",
      )[index];
      heading?.scrollIntoView({ behavior: "smooth", block: "start" });
    },
    revealMatch(source, match) {
      const root = wrapRef.current;
      if (!root) {
        return;
      }
      revealMarkdownMatch(root, source, match);
    },
  }), []);

  const emit = (vditor: Vditor) => {
    const root = wrapRef.current;
    if (!root) {
      return;
    }
    const markdown = markdownFromVditor(vditor, root);
    markdownRef.current = markdown;
    onChangeRef.current(markdown);
    void rewriteLocalImages(root, dirRef.current, workspacePathRef.current);
  };

  useEffect(() => {
    const shell = hostRef.current;
    if (!shell) {
      return;
    }

    let cancelled = false;
    let vditor: Vditor | null = null;
    const mount = document.createElement("div");
    mount.className = "vd-vditor vditor titlebar-no-drag h-full";
    shell.replaceChildren(mount);
    setStatus("loading");
    readyRef.current = false;
    silentRef.current = true;

    void import("vditor")
      .then(({ default: Vditor }) => {
        if (cancelled) {
          return;
        }
        vditor = new Vditor(mount, {
          cdn: VDITOR_CDN,
          cache: { enable: false },
          height: "100%",
          width: "auto",
          lang: vditorLang(language),
          mode: "wysiwyg",
          theme: vditorTheme(themeRef.current),
          icon: "ant",
          value: markdownRef.current,
          placeholder: "",
          toolbar: vditorToolbar({
            localTip: t("editor.importLocalImage"),
            remoteTip: t("editor.importRemoteImage"),
            importLocal: () => imageActionsRef.current.chooseLocalImage(),
            importRemote: () => imageActionsRef.current.askForRemoteImage(),
          }),
          toolbarConfig: { hide: !editableRef.current, pin: true },
          customWysiwygToolbar: tagWysiwygPopover,
          counter: { enable: false },
          outline: { enable: false, position: "left" },
          resize: { enable: false, position: "bottom" },
          preview: {
            mode: "editor",
            maxWidth: VDITOR_DISABLE_WIDTH_PADDING,
            hljs: {
              enable: true,
              lineNumber: false,
              style: vditorCodeTheme(themeRef.current),
            },
            theme: {
              current: VDITOR_CONTENT_THEME,
              path: VDITOR_CONTENT_THEME_PATH,
            },
            markdown: {
              toc: false,
              mark: true,
            },
            parse(element) {
              void rewriteLocalImages(element, dirRef.current, workspacePathRef.current);
            },
          },
          link: {
            isOpen: false,
            click(el) {
              const href = linkHref(el);
              if (!href || href.startsWith("#") || href.toLowerCase().startsWith("javascript:")) {
                return;
              }
              const event = window.event;
              const modified =
                event instanceof MouseEvent && (event.metaKey || event.ctrlKey || event.button === 1);
              if (!editableRef.current || modified) {
                void AppService.OpenURL(href).catch((err) => {
                  console.error("open url failed", err);
                });
              }
            },
          },
          after: () => {
            if (cancelled) {
              safeDestroy(vditor);
              return;
            }
            readyRef.current = true;
            vdRef.current = vditor;
            vditor?.setTheme(
              vditorTheme(themeRef.current),
              VDITOR_CONTENT_THEME,
              vditorCodeTheme(themeRef.current),
              VDITOR_CONTENT_THEME_PATH,
            );
            if (!editableRef.current) {
              vditor?.disabled();
            }
            setTaskCheckboxesLocked(mount, !editableRef.current);
            void rewriteLocalImages(mount, dirRef.current, workspacePathRef.current);
            setStatus("ready");
            window.setTimeout(() => {
              silentRef.current = false;
            }, 0);
          },
          input: () => {
            if (silentRef.current || !vditor) {
              return;
            }
            emit(vditor);
          },
          blur: () => {
            if (silentRef.current || !vditor) {
              return;
            }
            emit(vditor);
          },
        });
      })
      .catch((err) => {
        console.error("vditor load failed", err);
        if (!cancelled) {
          setStatus("error");
        }
      });

    return () => {
      cancelled = true;
      readyRef.current = false;
      silentRef.current = true;
      vdRef.current = null;
      safeDestroy(vditor);
      mount.remove();
    };
  }, [path, language]);

  useEffect(() => {
    const vditor = vdRef.current;
    if (!vditor || !readyRef.current) {
      return;
    }
    vditor.setTheme(
      vditorTheme(resolvedTheme),
      VDITOR_CONTENT_THEME,
      vditorCodeTheme(resolvedTheme),
      VDITOR_CONTENT_THEME_PATH,
    );
  }, [resolvedTheme]);

  useEffect(() => {
    const vditor = vdRef.current;
    if (!vditor || !readyRef.current) {
      return;
    }
    if (editable) {
      vditor.enable();
      vditor.updateToolbarConfig({ hide: false, pin: true });
      setTaskCheckboxesLocked(wrapRef.current, false);
      return;
    }
    vditor.disabled();
    vditor.updateToolbarConfig({ hide: true, pin: true });
    setTaskCheckboxesLocked(wrapRef.current, true);
  }, [editable]);

  useEffect(() => {
    const root = wrapRef.current;
    if (!root) {
      return;
    }
    const blockToggle = (event: Event) => {
      if (editableRef.current || !isTaskCheckbox(event.target)) {
        return;
      }
      event.preventDefault();
      event.stopImmediatePropagation();
    };
    const blockCodeExpand = (event: Event) => {
      if (editableRef.current || !(event.target instanceof Element)) {
        return;
      }
      if (event.target.closest(".vditor-copy")) {
        return;
      }
      if (event.target.closest(".vditor-wysiwyg__preview")) {
        event.stopImmediatePropagation();
      }
    };
    const blockKey = (event: KeyboardEvent) => {
      if (editableRef.current || !isTaskCheckbox(event.target)) {
        return;
      }
      if (event.key !== " " && event.key !== "Enter") {
        return;
      }
      event.preventDefault();
      event.stopImmediatePropagation();
    };
    const opts: AddEventListenerOptions = { capture: true };
    root.addEventListener("click", blockToggle, opts);
    root.addEventListener("click", blockCodeExpand, opts);
    root.addEventListener("pointerdown", blockToggle, opts);
    root.addEventListener("mousedown", blockToggle, opts);
    root.addEventListener("change", blockToggle, opts);
    root.addEventListener("keydown", blockKey, opts);
    const observer = new MutationObserver(() => {
      if (!editableRef.current) {
        setTaskCheckboxesLocked(root, true);
      }
    });
    observer.observe(root, { childList: true, subtree: true });
    const unpin = pinEditorScroll(root);
    return () => {
      unpin();
      root.removeEventListener("click", blockToggle, opts);
      root.removeEventListener("click", blockCodeExpand, opts);
      root.removeEventListener("pointerdown", blockToggle, opts);
      root.removeEventListener("mousedown", blockToggle, opts);
      root.removeEventListener("change", blockToggle, opts);
      root.removeEventListener("keydown", blockKey, opts);
      observer.disconnect();
    };
  }, []);

  useEffect(() => {
    const root = wrapRef.current;
    if (!root) {
      return;
    }

    let selected: HTMLImageElement | null = null;
    const controls = document.createElement("div");
    controls.className = "vd-image-controls";
    controls.hidden = true;
    const actions = document.createElement("div");
    actions.className = "vd-image-align";
    const alignments = [
      ["left", t("editor.imageAlignLeft"), "⇤"],
      ["center", t("editor.imageAlignCenter"), "↔"],
      ["right", t("editor.imageAlignRight"), "⇥"],
    ] as const;
    for (const [align, title, text] of alignments) {
      const button = document.createElement("button");
      button.type = "button";
      button.title = title;
      button.textContent = text;
      button.dataset.align = align;
      actions.appendChild(button);
    }
    const handle = document.createElement("button");
    handle.type = "button";
    handle.className = "vd-image-resize";
    handle.title = t("style.image");
    controls.append(actions, handle);
    root.appendChild(controls);

    const position = () => {
      if (!selected || !selected.isConnected || !editableRef.current) {
        controls.hidden = true;
        return;
      }
      const imageRect = selected.getBoundingClientRect();
      const rootRect = root.getBoundingClientRect();
      controls.hidden = false;
      controls.style.left = `${imageRect.left - rootRect.left}px`;
      controls.style.top = `${imageRect.top - rootRect.top}px`;
      controls.style.width = `${imageRect.width}px`;
      controls.style.height = `${imageRect.height}px`;
      actions.querySelectorAll("button").forEach((button) => {
        button.classList.toggle(
          "is-active",
          button.getAttribute("data-align") === parseImageMetadata(selected ? imageMarkdownSource(selected) : "").align,
        );
      });
    };

    const commitImage = () => {
      const vditor = vdRef.current;
      if (vditor) {
        emit(vditor);
      }
      void rewriteLocalImages(root, dirRef.current, workspacePathRef.current).then(position);
    };

    const selectImage = (event: MouseEvent) => {
      if (!editableRef.current || !(event.target instanceof HTMLImageElement)) {
        if (!(event.target instanceof Node) || !controls.contains(event.target)) {
          selected = null;
          controls.hidden = true;
        }
        return;
      }
      selected = event.target;
      applyImageMetadata(selected);
      position();
    };

    const setAlignment = (event: MouseEvent) => {
      const button = (event.target as Element | null)?.closest<HTMLButtonElement>("button[data-align]");
      if (!selected || !button) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      const align = button.dataset.align as "left" | "center" | "right";
      const metadata = parseImageMetadata(imageMarkdownSource(selected));
      setImageMarkdownSource(selected, formatImageMetadata(metadata, { align }));
      applyImageMetadata(selected);
      commitImage();
      position();
    };

    const startResize = (event: PointerEvent) => {
      if (!selected || event.button !== 0) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      const startX = event.clientX;
      const startRect = selected.getBoundingClientRect();
      const ratio = startRect.height / Math.max(startRect.width, 1);
      handle.setPointerCapture(event.pointerId);
      const move = (next: PointerEvent) => {
        const rootWidth = root.querySelector<HTMLElement>(".vditor-reset")?.clientWidth ?? root.clientWidth;
        const width = Math.round(Math.min(rootWidth, Math.max(32, startRect.width + next.clientX - startX)));
        const height = Math.round(width * ratio);
        if (selected) {
          selected.style.width = `${width}px`;
          selected.style.height = `${height}px`;
          position();
        }
      };
      const end = (next: PointerEvent) => {
        handle.releasePointerCapture(next.pointerId);
        handle.removeEventListener("pointermove", move);
        handle.removeEventListener("pointerup", end);
        handle.removeEventListener("pointercancel", end);
        if (!selected) {
          return;
        }
        const rect = selected.getBoundingClientRect();
        const metadata = parseImageMetadata(imageMarkdownSource(selected));
        setImageMarkdownSource(
          selected,
          formatImageMetadata(metadata, { width: Math.round(rect.width), height: Math.round(rect.height) }),
        );
        applyImageMetadata(selected);
        commitImage();
      };
      handle.addEventListener("pointermove", move);
      handle.addEventListener("pointerup", end);
      handle.addEventListener("pointercancel", end);
    };

    /* Vditor 自带的粘贴/拖放处理会把图片内联成 data URL，所以这些监听器跑在捕获
       阶段，让它根本拿不到这些文件。 */
    const importImages = (event: Event, files: File[]) => {
      if (!editableRef.current || files.length === 0) {
        return;
      }
      event.preventDefault();
      event.stopImmediatePropagation();
      void (async () => {
        for (const file of files) {
          await insertImportedImage(file);
        }
      })();
    };

    const pasteImages = (event: ClipboardEvent) =>
      importImages(event, collapseDuplicateFormats(imageFiles(event.clipboardData)));
    const dropImages = (event: DragEvent) => importImages(event, imageFiles(event.dataTransfer));

    const reposition = () => window.requestAnimationFrame(position);
    root.addEventListener("click", selectImage, true);
    root.addEventListener("paste", pasteImages, true);
    root.addEventListener("drop", dropImages, true);
    root.addEventListener("scroll", reposition, true);
    window.addEventListener("resize", reposition);
    actions.addEventListener("click", setAlignment);
    handle.addEventListener("pointerdown", startResize);
    return () => {
      root.removeEventListener("click", selectImage, true);
      root.removeEventListener("paste", pasteImages, true);
      root.removeEventListener("drop", dropImages, true);
      root.removeEventListener("scroll", reposition, true);
      window.removeEventListener("resize", reposition);
      actions.removeEventListener("click", setAlignment);
      handle.removeEventListener("pointerdown", startResize);
      controls.remove();
    };
  }, [t]);

  useEffect(() => {
    const root = wrapRef.current;
    if (!root) {
      return;
    }
    let lastSignature = "";
    const collectHeadings = () => {
      const headings = Array.from(
        root.querySelectorAll<HTMLElement>(
          ".vditor-wysiwyg h1, .vditor-wysiwyg h2, .vditor-wysiwyg h3, .vditor-wysiwyg h4, .vditor-wysiwyg h5, .vditor-wysiwyg h6",
        ),
        (element) => ({
          level: Number(element.tagName.slice(1)),
          text: (element.textContent || "").replace(/\u200b/g, "").trim(),
        }),
      ).filter((heading) => heading.text);
      const signature = JSON.stringify(headings);
      if (signature !== lastSignature) {
        lastSignature = signature;
        onHeadingsChangeRef.current?.(headings);
      }
    };
    const observer = new MutationObserver(collectHeadings);
    observer.observe(root, { childList: true, subtree: true, characterData: true });
    collectHeadings();
    return () => {
      observer.disconnect();
      onHeadingsChangeRef.current?.([]);
    };
  }, [path]);

  useEffect(() => {
    setRemoteOpen(false);
    setImportError("");
  }, [path, editable]);

  return (
    <div
      ref={wrapRef}
      className={cn(
        "relative h-full min-h-0",
        editable ? "bg-[var(--vd-bg)] vd-editable" : "bg-[var(--vd-bg-subtle)] vd-readonly",
      )}
      style={{ ["--vd-editor-width" as string]: editorWidth }}
    >
      {status !== "ready" && (
        <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center text-sm text-[var(--vd-fg-subtle)]">
          {status === "error" ? t("editor.loadFailed") : t("editor.loading")}
        </div>
      )}
      <div ref={hostRef} className="h-full" />
      {importError && !remoteOpen && (
        <div
          role="alert"
          className="absolute right-3 bottom-3 z-20 flex max-w-[min(420px,80%)] items-start gap-2 rounded-lg border border-[var(--vd-border)] bg-[var(--vd-bg-muted)] px-3 py-2 text-xs leading-5 text-red-600 shadow-md"
        >
          <span className="min-w-0 break-words">{`${t("editor.imageImportFailed")}: ${importError}`}</span>
          <button
            type="button"
            aria-label={t("search.close")}
            className="shrink-0 text-[var(--vd-fg-muted)] hover:text-[var(--vd-fg)]"
            onClick={() => setImportError("")}
          >
            <CloseIcon className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
      {remoteOpen && editable && (
        <RemoteImageModal t={t} onCancel={() => setRemoteOpen(false)} onSubmit={insertRemoteImage} />
      )}
    </div>
  );
});

function clearSearchHits(root: HTMLElement) {
  root.querySelectorAll(".vd-search-hit").forEach((el) => {
    const parent = el.parentNode;
    if (!parent) {
      return;
    }
    parent.replaceChild(document.createTextNode(el.textContent ?? ""), el);
    parent.normalize();
  });
}

function visibleText(value: string) {
  return value.replace(/\u200b/g, "");
}

function countOccurrences(haystack: string, needle: string) {
  if (!needle) {
    return 0;
  }
  let count = 0;
  let from = 0;
  while (from <= haystack.length) {
    const at = haystack.indexOf(needle, from);
    if (at < 0) {
      break;
    }
    count += 1;
    from = at + Math.max(needle.length, 1);
  }
  return count;
}

function wrapRange(range: Range) {
  const mark = document.createElement("span");
  mark.className = "vd-search-hit";
  try {
    range.surroundContents(mark);
    return mark;
  } catch {
    const fragment = range.extractContents();
    mark.appendChild(fragment);
    range.insertNode(mark);
    return mark;
  }
}

function findTextOccurrence(root: HTMLElement, needle: string, occurrence: number): Range | null {
  if (!needle) {
    return null;
  }
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let seen = 0;
  while (walker.nextNode()) {
    const node = walker.currentNode as Text;
    const raw = node.data;
    const text = visibleText(raw);
    if (!text) {
      continue;
    }
    let from = 0;
    while (from < text.length) {
      const at = text.indexOf(needle, from);
      if (at < 0) {
        break;
      }
      if (seen === occurrence) {
        const start = mapVisibleIndex(raw, at);
        const end = mapVisibleIndex(raw, at + needle.length);
        const range = document.createRange();
        range.setStart(node, start);
        range.setEnd(node, end);
        return range;
      }
      seen += 1;
      from = at + needle.length;
    }
  }
  return null;
}

function mapVisibleIndex(raw: string, visibleIndex: number) {
  let visible = 0;
  for (let i = 0; i < raw.length; i++) {
    if (visible >= visibleIndex) {
      return i;
    }
    if (raw[i] !== "\u200b") {
      visible += 1;
    }
  }
  return raw.length;
}

function revealMarkdownMatch(root: HTMLElement, source: string, match: SearchMatch) {
  clearSearchHits(root);
  const wysiwyg = root.querySelector<HTMLElement>(".vditor-wysiwyg") ?? root;
  const start = utf16FromUtf8Offset(source, match.startByte);
  const needle = match.match || source.slice(start, utf16FromUtf8Offset(source, match.endByte));
  const occurrence = countOccurrences(source.slice(0, start), needle);
  let range = findTextOccurrence(wysiwyg, needle, occurrence);
  if (!range && needle) {
    range = findTextOccurrence(wysiwyg, needle, 0);
  }
  if (!range) {
    const snippet = (match.lineText || "").replace(/[#*_`[\]()>|-]/g, "").trim();
    if (snippet) {
      range = findTextOccurrence(wysiwyg, snippet.slice(0, Math.min(snippet.length, 24)), 0);
    }
  }
  if (!range) {
    return;
  }
  const mark = wrapRange(range);
  mark.scrollIntoView({ behavior: "smooth", block: "center" });
}
