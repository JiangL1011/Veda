export type DocTabs = {
  /* 标签栏的显示顺序，也是保存到工作区布局里的顺序。内容面板不按它排列：
     带 key 的子元素换序会让 React 重新插入 DOM 节点，从而重置其中所有可滚动
     元素的滚动位置，所以面板固定按首次打开的顺序渲染（见 EditorPane 的 loaded）。 */
  order: string[];
  /* 最近打开的排在最前，只用来决定淘汰哪一个。 */
  recent: string[];
};

export const NO_DOC_TABS: DocTabs = { order: [], recent: [] };

export function restoreDocTabs(paths: string[], active: string): DocTabs {
  const order: string[] = [];
  const seen = new Set<string>();
  for (const item of [...paths, active]) {
    if (!item || seen.has(item)) {
      continue;
    }
    seen.add(item);
    order.push(item);
  }
  if (order.length === 0) {
    return NO_DOC_TABS;
  }
  const recent = active && order.includes(active) ? [active, ...order.filter((item) => item !== active)] : [...order];
  return { order, recent };
}

export function dropDocTabs(tabs: DocTabs, path: string): DocTabs {
  if (!path) {
    return tabs;
  }
  const keep = (item: string) => item !== path && !item.startsWith(path + "/") && !item.startsWith(path + "\\");
  const order = tabs.order.filter(keep);
  const recent = tabs.recent.filter(keep);
  if (samePaths(order, tabs.order) && samePaths(recent, tabs.recent)) {
    return tabs;
  }
  return { order, recent };
}

export function openDocTab(tabs: DocTabs, path: string): DocTabs {
  if (!path) {
    return tabs;
  }
  const recent = [path, ...tabs.recent.filter((item) => item !== path)];
  const order = tabs.order.includes(path) ? tabs.order : [...tabs.order, path];
  if (order === tabs.order && samePaths(recent, tabs.recent)) {
    return tabs;
  }
  return { order, recent };
}

export function pruneDocTabs(
  tabs: DocTabs,
  keep: string,
  maxTabs: number,
): { tabs: DocTabs; dropped: string[] } {
  const order = [...tabs.order];
  const recent = [...tabs.recent];
  const dropped: string[] = [];
  const limit = Math.max(1, Math.floor(maxTabs));

  while (order.length > limit) {
    let i = recent.length - 1;
    while (i >= 0 && recent[i] === keep) {
      i -= 1;
    }
    if (i < 0) {
      break;
    }
    const victim = recent.splice(i, 1)[0];
    const at = order.indexOf(victim);
    if (at >= 0) {
      order.splice(at, 1);
    }
    dropped.push(victim);
  }

  if (dropped.length === 0) {
    return { tabs, dropped };
  }
  return { tabs: { order, recent }, dropped };
}

export function closeDocTab(tabs: DocTabs, path: string): DocTabs {
  if (!tabs.order.includes(path)) {
    return tabs;
  }
  return {
    order: tabs.order.filter((item) => item !== path),
    recent: tabs.recent.filter((item) => item !== path),
  };
}

/* 把标签拖到目标位置。index 是移除被拖标签后其余标签之间的插入位置，
   与拖拽时按指针位置算出的槽位一致。 */
export function moveDocTab(tabs: DocTabs, path: string, index: number): DocTabs {
  if (!tabs.order.includes(path)) {
    return tabs;
  }
  const order = tabs.order.filter((item) => item !== path);
  const at = Math.max(0, Math.min(order.length, index));
  order.splice(at, 0, path);
  if (samePaths(order, tabs.order)) {
    return tabs;
  }
  return { order, recent: tabs.recent };
}

export function rewriteDocTabs(tabs: DocTabs, from: string, to: string): DocTabs {
  if (!from || from === to) {
    return tabs;
  }
  const order = rewritePaths(tabs.order, from, to);
  const recent = rewritePaths(tabs.recent, from, to);
  if (samePaths(order, tabs.order) && samePaths(recent, tabs.recent)) {
    return tabs;
  }
  return { order, recent };
}

export function rewriteDocPath(path: string, from: string, to: string): string {
  if (path === from) {
    return to;
  }
  const sep = from.includes("\\") && !from.includes("/") ? "\\" : "/";
  return path.startsWith(from + sep) ? to + path.slice(from.length) : path;
}

function rewritePaths(paths: string[], from: string, to: string): string[] {
  const next: string[] = [];
  const seen = new Set<string>();
  for (const item of paths) {
    const path = rewriteDocPath(item, from, to);
    if (seen.has(path)) {
      continue;
    }
    seen.add(path);
    next.push(path);
  }
  return next;
}

function samePaths(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((item, i) => item === b[i]);
}
