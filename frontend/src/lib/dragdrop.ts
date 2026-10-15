import type { FileEntry } from "../../bindings/veda/models";

/* 拖拽进来的文件在等着用户选择处理方式。 */
export type FileDropPrompt = {
  files: string[];
  destDir: string;
  workspace: string;
};

/* Go 侧导入完成后派发回来的结果。 */
export type FileDropResult = {
  destDir: string;
  workspace: string;
  action: string;
  imported: FileEntry[] | null;
  errors: string[] | null;
};

/* 前端标在落点元素上的属性。Go 通过 DropTargetDetails 读取 data-drop-dir，
   另外两个只在界面里用。 */
export const DROP_TARGET_ATTRIBUTE = "data-file-drop-target";
export const DROP_DIR_ATTRIBUTE = "data-drop-dir";
export const DROP_LABEL_ATTRIBUTE = "data-drop-label";

export const FILE_DROP_PROMPT_EVENT = "veda:file-drop";
export const FILE_DROP_DONE_EVENT = "veda:file-drop-done";

type DragState = {
  active: boolean;
  label: string;
};

let dragState: DragState = { active: false, label: "" };
const listeners = new Set<() => void>();

function publish(next: DragState) {
  if (next.active === dragState.active && next.label === dragState.label) {
    return;
  }
  dragState = next;
  for (const listener of [...listeners]) {
    listener();
  }
}

export function subscribeDragState(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function readDragState() {
  return dragState;
}

/* 强制收起拖拽提示。导入流程一开始就说明这次拖拽已经结束，用它兜住任何
   平台差异导致的“提示留在屏幕上”。 */
export function resetDragState() {
  publish({ active: false, label: "" });
}

/* 指针下面最近的落点元素：目录树的某一行，或者编辑区 / 整个窗口。 */
export function dropTargetAt(x: number, y: number): { dir: string; label: string } {
  const el = document.elementFromPoint(x, y);
  const target = el?.closest(`[${DROP_TARGET_ATTRIBUTE}]`) ?? null;
  if (!target) {
    return { dir: "", label: "" };
  }
  return {
    dir: target.getAttribute(DROP_DIR_ATTRIBUTE) ?? "",
    label: target.getAttribute(DROP_LABEL_ATTRIBUTE) ?? "",
  };
}

/* 运行时会把 file-drop-target-active 加在当前落点元素上，拖拽一结束就摘掉。
   它比我们自己的状态更早、也更可靠地反映“这一轮拖拽还在不在”。 */
function highlightedDropTarget(): Element | null {
  return document.querySelector(`[${DROP_TARGET_ATTRIBUTE}].file-drop-target-active`);
}

type RuntimeDragHandlers = {
  handleDragEnter?: () => void;
  handleDragOver?: (x: number, y: number) => void;
  handleDragLeave?: () => void;
  handlePlatformFileDrop?: (files: string[], x: number, y: number) => void;
};

function runtimeHandlers(): RuntimeDragHandlers | null {
  const wails = (window as unknown as { _wails?: RuntimeDragHandlers })._wails;
  return wails ?? null;
}

/* macOS / Linux 上原生层拦下了 DOM 拖拽事件，由 Go 调用 window._wails 上的
   这几个钩子；Windows 上则是页面自己收到 dragenter/dragover/drop。两条路径都
   接上，界面才能在整个拖拽过程中给出反馈。 */
export function installDragTracking() {
  const runtime = runtimeHandlers();
  const originals: RuntimeDragHandlers = {
    handleDragEnter: runtime?.handleDragEnter,
    handleDragOver: runtime?.handleDragOver,
    handleDragLeave: runtime?.handleDragLeave,
    handlePlatformFileDrop: runtime?.handlePlatformFileDrop,
  };

  if (runtime) {
    runtime.handleDragEnter = () => {
      originals.handleDragEnter?.();
      // 新的一轮拖拽从空标签开始，避免沿用上一次的提示。
      publish({ active: true, label: "" });
    };
    runtime.handleDragOver = (x: number, y: number) => {
      originals.handleDragOver?.(x, y);
      /* macOS 上 dragOver 是 50ms 节流后延迟派发的，拖拽结束（松手或移出窗口）
         之后可能还会迟到一次。运行时的原生拖拽状态此时已经清掉，所以它不会再加
         高亮；这里同样只在拖拽确实还在进行时才更新提示，否则提示会重新冒出来并且
         再也没有事件把它收回去。 */
      if (!dragState.active && !highlightedDropTarget()) {
        return;
      }
      const target = dropTargetAt(x, y);
      publish({ active: true, label: target.label });
    };
    runtime.handleDragLeave = () => {
      originals.handleDragLeave?.();
      publish({ active: false, label: "" });
    };
    runtime.handlePlatformFileDrop = (files: string[], x: number, y: number) => {
      originals.handlePlatformFileDrop?.(files, x, y);
      publish({ active: false, label: "" });
    };
  }

  const onDomDragOver = (event: DragEvent) => {
    if (!event.dataTransfer?.types.includes("Files")) {
      return;
    }
    const target = dropTargetAt(event.clientX, event.clientY);
    publish({ active: true, label: target.label });
  };
  const onDomDragLeave = (event: DragEvent) => {
    if (!event.dataTransfer?.types.includes("Files") || event.relatedTarget !== null) {
      return;
    }
    publish({ active: false, label: "" });
  };
  const onDomDrop = () => publish({ active: false, label: "" });

  document.addEventListener("dragover", onDomDragOver, true);
  document.addEventListener("dragleave", onDomDragLeave, true);
  document.addEventListener("drop", onDomDrop, true);
  document.addEventListener("dragend", onDomDrop, true);

  return () => {
    if (runtime) {
      runtime.handleDragEnter = originals.handleDragEnter;
      runtime.handleDragOver = originals.handleDragOver;
      runtime.handleDragLeave = originals.handleDragLeave;
      runtime.handlePlatformFileDrop = originals.handlePlatformFileDrop;
    }
    document.removeEventListener("dragover", onDomDragOver, true);
    document.removeEventListener("dragleave", onDomDragLeave, true);
    document.removeEventListener("drop", onDomDrop, true);
    document.removeEventListener("dragend", onDomDrop, true);
    publish({ active: false, label: "" });
  };
}
