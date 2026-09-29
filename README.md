# Veda

基于 Wails v3 的所见即所得 Markdown 编辑器。前端为 React + TypeScript + Tailwind CSS，编辑器使用 [Vditor](https://github.com/vanessa219/vditor)。

## 开发

```bash
wails3 dev
```

应用数据保存在用户目录下的 `.veda/`：全局 `session.json`、`settings.json` 和 `workspace.json`（工作区路径到 uuid 的映射）。每个工作区的布局、覆盖设置和锁定状态写在 `.veda/{uuid}/`，不再使用工作区内的 `.veda` 目录。首次以 Veda 启动时会自动把旧版 Marknote 的 `~/.marknote/` 数据迁移过来。
