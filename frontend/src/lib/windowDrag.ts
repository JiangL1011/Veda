/* 拖动窗口时指针会滑过正文，WKWebView 默认把光标换成 I 型文本光标。
   在拖拽区按下时先锁住光标，松开或窗口失焦时再恢复。 */
export function installWindowDragCursor() {
  const root = document.documentElement;
  const setDragging = (active: boolean) => {
    if (root.classList.contains("vd-window-dragging") === active) {
      return;
    }
    root.classList.toggle("vd-window-dragging", active);
  };

  window.addEventListener(
    "mousedown",
    (event) => {
      if (event.button !== 0) {
        return;
      }
      const target = event.target instanceof Element ? event.target : null;
      const draggable =
        !!target && getComputedStyle(target).getPropertyValue("--wails-draggable").trim() === "drag";
      setDragging(draggable);
    },
    true,
  );

  const stop = () => setDragging(false);
  window.addEventListener("mouseup", stop, true);
  window.addEventListener("blur", stop);
  /* 原生拖拽期间万一收不到 mouseup，下一次移动时按键已松开就补一次复位。 */
  window.addEventListener(
    "mousemove",
    (event) => {
      if (event.buttons === 0) {
        stop();
      }
    },
    true,
  );
}
