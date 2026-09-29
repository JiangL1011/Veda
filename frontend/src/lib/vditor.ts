import { AppService } from "./api";
import type { Language } from "./settings";

export const VDITOR_CDN = `${import.meta.env.BASE_URL}vditor`.replace(/\/+$/, "") || "/vditor";
export const VDITOR_CONTENT_THEME = "veda";
export const VDITOR_CONTENT_THEME_PATH =
  `${import.meta.env.BASE_URL}md-themes`.replace(/\/+$/, "") || "/md-themes";
/* Vditor 用 (paneWidth - preview.maxWidth) / 2 的内边距来把正文列居中。 */
export const VDITOR_DISABLE_WIDTH_PADDING = 100000;

/* Vditor 自带的图标是填充式的，我们的是描边式的，参见 index.css 里的 .vd-toolbar-icon。 */
function toolbarIcon(body: string) {
  return `<svg class="vd-toolbar-icon" viewBox="0 0 24 24">${body}</svg>`;
}

const LOCAL_IMAGE_ICON = toolbarIcon(
  '<rect x="3" y="4" width="18" height="16" rx="2" /><circle cx="9" cy="10" r="1.6" /><path d="m21 15-5-5-11 9" />',
);

const REMOTE_IMAGE_ICON = toolbarIcon(
  '<circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" />',
);

export type ImageToolbarActions = {
  localTip: string;
  remoteTip: string;
  importLocal: () => void;
  importRemote: () => void;
};

export function vditorToolbar(image: ImageToolbarActions): Array<string | IMenuItem> {
  return [
    "headings",
    "bold",
    "italic",
    "strike",
    "link",
    "|",
    {
      name: "vd-local-image",
      tip: image.localTip,
      tipPosition: "s",
      icon: LOCAL_IMAGE_ICON,
      click: () => image.importLocal(),
    },
    {
      name: "vd-remote-image",
      tip: image.remoteTip,
      tipPosition: "s",
      icon: REMOTE_IMAGE_ICON,
      click: () => image.importRemote(),
    },
    "|",
    "list",
    "ordered-list",
    "check",
    "outdent",
    "indent",
    "|",
    "quote",
    "line",
    "code",
    "inline-code",
    "table",
    "|",
    "undo",
    "redo",
  ];
}

/* 所有块类型共用同一个浮层元素（表格保留自带的，图片用我们自己的对齐控件），
   而这个钩子是 Vditor 唯一会告诉我们它即将展示哪一种块的地方。 */
const HIDDEN_POPOVERS = new Set<TWYSISYGToolbar>(["image", "heading", "block", "blockquote", "li"]);

export function tagWysiwygPopover(type: TWYSISYGToolbar, popover: HTMLElement) {
  popover.dataset.mnPopoverHidden = HIDDEN_POPOVERS.has(type) ? "true" : "false";
}

export function vditorLang(language: Language): "zh_CN" | "en_US" {
  return language === "en" ? "en_US" : "zh_CN";
}

export function vditorTheme(theme: "light" | "dark"): "classic" | "dark" {
  return theme === "dark" ? "dark" : "classic";
}

export function vditorCodeTheme(theme: "light" | "dark"): string {
  return theme === "dark" ? "github-dark" : "github";
}

export function vditorPreviewOptions(theme: "light" | "dark"): IPreviewOptions {
  return {
    cdn: VDITOR_CDN,
    mode: theme,
    theme: {
      current: VDITOR_CONTENT_THEME,
      path: VDITOR_CONTENT_THEME_PATH,
    },
    hljs: {
      enable: true,
      lineNumber: false,
      style: vditorCodeTheme(theme),
    },
  };
}

const EDIT_KEYS = new Set(["Enter", "Tab", "Backspace", "Delete"]);
/* 这个时长足以覆盖一次按键触发的重新渲染，又短到不会妨碍用户的下一个操作。 */
const PIN_MS = 300;
const CARET_MARGIN = 24;

function caretRect(range: Range): DOMRect | null {
  const caret = range.cloneRange();
  caret.collapse(false);
  const rect = caret.getClientRects()[0] || caret.getBoundingClientRect();
  if (rect && (rect.left || rect.top || rect.width || rect.height)) {
    return rect as DOMRect;
  }
  const node = caret.startContainer;
  const el = node.nodeType === Node.ELEMENT_NODE ? (node as Element) : node.parentElement;
  return el ? el.getBoundingClientRect() : null;
}

function revealCaret(scroller: HTMLElement, range: Range) {
  const caret = caretRect(range);
  if (!caret) {
    return;
  }
  const box = scroller.getBoundingClientRect();
  const margin = Math.min(CARET_MARGIN, box.width / 4);
  if (caret.left < box.left + margin) {
    scroller.scrollLeft -= box.left + margin - caret.left;
  } else if (caret.right > box.right - margin) {
    scroller.scrollLeft += caret.right - (box.right - margin);
  }
}

/* 内容主题把表格渲染成 `display: block; overflow: auto`，所以过宽的表格是在
   自身内部滚动，而不是带动整个编辑器滚动。 */
function scrollBoxAtCaret(root: HTMLElement): { box: HTMLElement; range: Range } | null {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0) {
    return null;
  }
  const range = selection.getRangeAt(0);
  const start = range.startContainer;
  if (!root.contains(start)) {
    return null;
  }
  let el = start.nodeType === Node.ELEMENT_NODE ? (start as HTMLElement) : start.parentElement;
  while (el && root.contains(el)) {
    if (el.scrollWidth > el.clientWidth + 1) {
      const overflow = getComputedStyle(el).overflowX;
      if (overflow === "auto" || overflow === "scroll") {
        return { box: el, range };
      }
    }
    el = el.parentElement;
  }
  return null;
}

/* 每次输入 Vditor 都会重建被编辑的块（`blockElement.outerHTML = html`），整张表格
   也随之被替换。新元素的 scrollLeft 从 0 开始，于是恢复光标位置时视图会被拖到
   最右边。这里在多次编辑之间固定住滚动偏移，只有当光标将要移出可视区域时才
   跟随它。 */
export function pinEditorScroll(root: HTMLElement): () => void {
  let left: number | null = null;
  let pinnedUntil = 0;

  const pin = () => {
    const found = scrollBoxAtCaret(root);
    if (!found) {
      return;
    }
    left = found.box.scrollLeft;
    pinnedUntil = performance.now() + PIN_MS;
  };

  const release = () => {
    pinnedUntil = 0;
  };

  const restore = () => {
    if (left === null || performance.now() > pinnedUntil) {
      return;
    }
    const found = scrollBoxAtCaret(root);
    if (!found || found.box.scrollLeft === left) {
      return;
    }
    found.box.scrollLeft = left;
    revealCaret(found.box, found.range);
    left = found.box.scrollLeft;
  };

  const onKeyDown = (event: KeyboardEvent) => {
    if (EDIT_KEYS.has(event.key)) {
      pin();
    }
  };

  const capture: AddEventListenerOptions = { capture: true, passive: true };
  const bubble: AddEventListenerOptions = { passive: true };
  /* 捕获阶段早于 Vditor 自己挂在编辑器元素上的处理函数，冒泡阶段则在块被重建之后；
     而 scroll 这一遍用来兜住 WebKit 推迟到下一帧才执行的光标滚动定位。 */
  root.addEventListener("beforeinput", pin, capture);
  root.addEventListener("compositionupdate", pin, capture);
  root.addEventListener("keydown", onKeyDown, capture);
  root.addEventListener("scroll", restore, capture);
  root.addEventListener("wheel", release, capture);
  root.addEventListener("pointerdown", release, capture);
  root.addEventListener("input", restore, bubble);
  root.addEventListener("compositionend", restore, bubble);
  root.addEventListener("keyup", restore, bubble);
  return () => {
    root.removeEventListener("beforeinput", pin, capture);
    root.removeEventListener("compositionupdate", pin, capture);
    root.removeEventListener("keydown", onKeyDown, capture);
    root.removeEventListener("scroll", restore, capture);
    root.removeEventListener("wheel", release, capture);
    root.removeEventListener("pointerdown", release, capture);
    root.removeEventListener("input", restore, bubble);
    root.removeEventListener("compositionend", restore, bubble);
    root.removeEventListener("keyup", restore, bubble);
  };
}

export function isRemoteMedia(src: string) {
  return /^(https?:|data:|blob:|\/__media\/)/i.test(src.trim());
}

export type ImageAlignment = "left" | "center" | "right";

export type ImageMetadata = {
  path: string;
  width?: number;
  height?: number;
  align: ImageAlignment;
  params: URLSearchParams;
  hash: string;
};

export function parseImageMetadata(source: string): ImageMetadata {
  const hashAt = source.indexOf("#");
  const hash = hashAt < 0 ? "" : source.slice(hashAt);
  const withoutHash = hashAt < 0 ? source : source.slice(0, hashAt);
  const queryAt = withoutHash.indexOf("?");
  const path = queryAt < 0 ? withoutHash : withoutHash.slice(0, queryAt);
  const params = new URLSearchParams(queryAt < 0 ? "" : withoutHash.slice(queryAt + 1));
  const number = (key: string) => {
    const value = Number(params.get(key));
    return Number.isFinite(value) && value > 0 ? Math.round(value) : undefined;
  };
  const rawAlign = params.get("align");
  return {
    path,
    width: number("with") ?? number("width"),
    height: number("height"),
    align: rawAlign === "left" || rawAlign === "right" ? rawAlign : "center",
    params,
    hash,
  };
}

/* 未传入的值沿用路径中已有的参数，因此修改其中一个属性不会把其他属性丢掉。 */
export function formatImageMetadata(
  current: ImageMetadata,
  values: { width?: number; height?: number; align?: ImageAlignment },
) {
  const params = new URLSearchParams(current.params);
  params.delete("width");
  const width = values.width ?? current.width;
  const height = values.height ?? current.height;
  if (width && width > 0) {
    params.set("with", String(Math.round(width)));
  } else {
    params.delete("with");
  }
  if (height && height > 0) {
    params.set("height", String(Math.round(height)));
  } else {
    params.delete("height");
  }
  const align = values.align ?? current.align;
  if (align === "center") {
    params.delete("align");
  } else {
    params.set("align", align);
  }
  const query = params.toString();
  return `${current.path}${query ? `?${query}` : ""}${current.hash}`;
}

export function imageMarkdownSource(img: HTMLImageElement) {
  return img.dataset.origin || img.getAttribute("data-src") || img.getAttribute("src") || "";
}

export function setImageMarkdownSource(img: HTMLImageElement, source: string) {
  if (img.dataset.origin) {
    img.dataset.origin = source;
  } else {
    img.setAttribute("src", source);
  }
  if (img.hasAttribute("data-src")) {
    img.setAttribute("data-src", source);
  }
}

function imageResourcePath(source: string) {
  return parseImageMetadata(source).path;
}

export function applyImageMetadata(img: HTMLImageElement) {
  const metadata = parseImageMetadata(imageMarkdownSource(img));
  img.style.width = metadata.width ? `${metadata.width}px` : "";
  img.style.height = metadata.height ? `${metadata.height}px` : "";
  img.style.maxWidth = "100%";
  img.style.display = "block";
  img.style.marginLeft = metadata.align === "right" ? "auto" : metadata.align === "center" ? "auto" : "0";
  img.style.marginRight = metadata.align === "left" ? "auto" : metadata.align === "center" ? "auto" : "0";
  img.dataset.mnImage = "true";
}

export async function rewriteLocalImages(root: ParentNode, dir: string, workspacePath = "") {
  const imgs = Array.from(root.querySelectorAll("img"));
  await Promise.all(
    imgs.map(async (img) => {
      applyImageMetadata(img);
      if (img.dataset.origin) {
        return;
      }
      const origin = imageMarkdownSource(img);
      if (!origin || isRemoteMedia(origin)) {
        return;
      }
      try {
        const abs = await AppService.ResolveImagePath(workspacePath, dir, imageResourcePath(origin));
        const url = await AppService.MediaURL(abs);
        img.dataset.origin = origin;
        img.setAttribute("src", url);
      } catch {
        /* 保留原始 src */
      }
    }),
  );
}

export function markdownFromVditor(vditor: { getValue: () => string }, root: ParentNode): string {
  const imgs = Array.from(root.querySelectorAll("img"));
  const restored: { img: HTMLImageElement; display: string }[] = [];
  for (const img of imgs) {
    const origin = img.dataset.origin;
    if (!origin) {
      continue;
    }
    restored.push({ img, display: img.getAttribute("src") || "" });
    img.setAttribute("src", origin);
  }
  try {
    return vditor.getValue();
  } finally {
    for (const item of restored) {
      item.img.setAttribute("src", item.display);
    }
  }
}
