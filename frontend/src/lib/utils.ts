export function cn(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(" ");
}

/* 标题栏需要给 macOS 的红绿灯按钮留出空间，而 Windows 和 Linux 的原生窗口
   控件则不需要。 */
export function hostPlatform(): "mac" | "windows" | "linux" {
  const ua = navigator.userAgent;
  if (ua.includes("Macintosh") || ua.includes("Mac OS X")) {
    return "mac";
  }
  if (ua.includes("Windows")) {
    return "windows";
  }
  return "linux";
}

export function shouldRestoreOnLaunch() {
  const params = new URLSearchParams(window.location.search);
  return params.get("restore") === "1";
}

/* 由“打开文件/文件夹”创建的窗口会把目标写在 URL 里，这样首次渲染前就能拿到它，
   而不必等待后续事件。 */
export function launchWorkspace(): unknown {
  const raw = new URLSearchParams(window.location.search).get("open");
  if (!raw) {
    return null;
  }
  try {
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function pathSep(path: string) {
  return path.includes("\\") && !path.includes("/") ? "\\" : "/";
}

export function isSameOrInside(parent: string, path: string) {
  return path === parent || path.startsWith(parent + pathSep(parent));
}

/* HTML 文件以源码方式编辑，只读时可以切换成渲染预览。 */
export function isHtmlFile(name: string) {
  return /\.(html?|xhtml)$/i.test(name);
}

export function relativePath(root: string, path: string) {
  if (path === root) {
    return ".";
  }
  const sep = pathSep(root);
  if (path.startsWith(root + sep)) {
    return path.slice(root.length + 1);
  }
  return path;
}

/* 路径的最后一段，用于把目录名展示给用户。 */
export function baseName(path: string) {
  const trimmed = path.replace(/[/\\]+$/, "");
  const parts = trimmed.split(/[/\\]/);
  return parts[parts.length - 1] || trimmed;
}

/* 路径所在的目录，保持原样的分隔符风格。 */
export function dirName(path: string) {
  const trimmed = path.replace(/[/\\]+$/, "");
  const index = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  if (index < 0) {
    return trimmed;
  }
  if (index === 0) {
    return trimmed.slice(0, 1);
  }
  return trimmed.slice(0, index);
}

export async function copyText(value: string) {
  try {
    await navigator.clipboard.writeText(value);
  } catch {
    const el = document.createElement("textarea");
    el.value = value;
    el.style.position = "fixed";
    el.style.left = "-9999px";
    document.body.appendChild(el);
    el.select();
    document.execCommand("copy");
    el.remove();
  }
}

export function rewritePath(oldPath: string, newPath: string, target: string) {
  if (target === oldPath) {
    return newPath;
  }
  if (target.startsWith(oldPath + pathSep(oldPath))) {
    return newPath + target.slice(oldPath.length);
  }
  return target;
}
