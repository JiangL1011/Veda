---
name: veda-release
description: 发布 Veda 新版本：同步版本号、提交并推送附注 tag、等 CI 构建、在 GitHub Release 页面写入三段式更新日志。当用户要求发版、发布新版本、推送 tag、写 release notes 时使用。
whenToUse: 用户说「发布 X.Y.Z」「发个版」「提交并推送一个 tag」「在 release 页面写更新日志」，或需要为已有 tag 补写 Release 正文时。
---

# Veda 发版流程

本仓库（`JiangL1011/Veda`）的发版固定为六步：**定版本号 → 同步打包资源 → 提交推送 main → 打并推送 tag → 等 CI → 写 Release 更新日志**。
安装包由 tag 触发的 GitHub Actions 构建产出并自动附到 Release 上，不需要本地打包。

## 前置检查

```bash
git status --short            # 必须干净；有未提交改动先问用户怎么处理
git fetch origin && git log --oneline -1 origin/main
gh auth status                # 必须是当前账号
gh api repos/JiangL1011/Veda --jq .permissions   # 需要 push 或 admin 为 true
go test ./...
```

- **`gh` 账号必须有写权限**（最容易踩的坑）。若 `permissions.push=false`，`gh release edit` / `gh release create` 会返回 **404 Not Found**（不是 403），很容易被误判成「Release 不存在」而反复重试。此时用 `gh auth login` 切到有写权限的账号再继续。
  注意 `git push` 走的是 `osxkeychain` 里保存的凭据，可能与 `gh` 的 token 不是同一个账号——**push 成功并不代表 `gh` 有写权限**。
- `go test ./...` 若报 `pattern ./...: open .../go-build/...: operation not permitted`，是沙箱不允许写 Go 构建缓存，改用
  `GOCACHE="$(pwd)/.task/gocache" go test ./...`，跑完删掉 `.task/gocache`。
- 网络偶发 `LibreSSL SSL_connect: SSL_ERROR_SYSCALL`，`git push` 与 `gh`/`curl` 命令都要套重试循环。

## 第 1 步：确定版本号

```bash
git tag --list 'v*' --sort=-v:refname | head -1   # 上一个版本
git log --oneline v<PREV>..HEAD                   # 本版改动范围
```

- **用户提供了版本号 → 直接用**，不要自作主张改。
- **用户没提供 → 先推断，并在动任何文件之前把推断的版本号和理由告诉用户，等用户同意后再继续。** 推断规则：
  - 有 `feat`（面向用户的新能力）→ 次版本号 +1：`0.2.1` → `0.3.0`
  - 只有 `fix` / `perf` / `docs` / `chore` / `ci` → 修订号 +1：`0.2.1` → `0.2.2`
  - 有破坏性变更（提交标题带 `!` 或正文含 `BREAKING CHANGE`）→ 主版本号 +1
  - 本仓库历史里也出现过「feat 与 fix 混批只升修订号」的情况，所以推断只是建议，**以用户确认的为准**；用户没明确表态就不要提交。

## 第 2 步：同步版本号到打包资源

```bash
NEW=0.2.2; OLD=0.2.1
grep -rn "$OLD" build/     # 先逐行确认每一处都是版本号，别误伤 10.2.1 这类数字
perl -pi -e "s/\Q$OLD\E/$NEW/g" \
  build/config.yml \
  build/darwin/Info.plist build/darwin/Info.dev.plist \
  build/windows/info.json build/windows/msix/app_manifest.xml \
  build/windows/msix/template.xml build/windows/nsis/wails_tools.nsh \
  build/windows/wails.exe.manifest \
  build/linux/nfpm/nfpm.yaml build/ios/Info.plist build/ios/Info.dev.plist
grep -rn "$NEW" build/     # 复核；顺带确认旧版本号已无残留
```

- `build/config.yml` 的 `info.version` 是应用「当前版本」的唯一来源（`main.go` 通过 `go:embed` 读取，`internal/version` 解析），也是 CI 解析版本的兜底。
- macOS `Info.plist` / `Info.dev.plist` 各有两处（`CFBundleShortVersionString`、`CFBundleVersion`）。
- Windows MSIX 清单是四段版本，替换后应变成 `X.Y.Z.0`。
- `build/linux`、`build/ios` 未纳入版本控制，但仍要保持本地一致。
- **不要改** `*_test.go` 里的 `Veda-0.2.0-*.dmg` 这类测试夹具、`frontend/package.json` 的 `0.0.0`，它们与发布版本无关。
- 版本号本身不用改 `internal/version`。

## 第 3 步：提交并推送到 main

commit 标题固定为 `chore(release): 版本号提升到 X.Y.Z`，正文说明同步了哪些文件（照抄上一版的结构即可）。

```bash
git add build/config.yml build/darwin/Info.plist build/darwin/Info.dev.plist \
        build/windows/info.json build/windows/msix/app_manifest.xml \
        build/windows/msix/template.xml build/windows/nsis/wails_tools.nsh \
        build/windows/wails.exe.manifest
git commit -F - <<'EOF'
chore(release): 版本号提升到 0.2.2

同步所有打包资源里的版本号，避免仓库内版本与 tag 不一致。
EOF
git push origin main
```

推送 `main` **不会**触发构建（工作流只在 `v*` tag、PR、手动触发时运行），构建只由下一步的 tag 触发。

## 第 4 步：打 tag 并推送

```bash
git tag -a v0.2.2 -m "Veda 0.2.2" -m "<一两句本版要点>"
git push origin v0.2.2
```

- 必须是**附注 tag**（`-a`），与既有 `v0.1.0`…`v0.2.1` 保持一致；tag 名带 `v` 前缀，应用内版本号不带。
- 建议等第 3 步的 main 推送完成后再推 tag，让 tag 指向已上远端的那次提交。

## 第 5 步：等 CI 构建完成

```bash
gh run list --limit 3
gh run view <run-id> --json status,conclusion -q '.status + " " + .conclusion'   # 轮询直到 completed
```

- **不要用 `gh run watch`**：网络一断它就退出，看起来像跑完了，实际的 conclusion 可能还没出；用上面这条轮询命令并套重试。
- 流水线顺序：`Resolve version` → macOS arm64/x64 与 Windows x64（并行）→ `Publish release`，耗时约 8–16 分钟。
- `release` job 依赖 macOS 与 Windows **全部成功**；任一失败就不会自动创建 Release，此时先把失败原因报给用户，别再谈写日志。
- Release 由 `softprops/action-gh-release` 自动创建（配置里带 `generate_release_notes: true`），正文是自动生成的，稍后会被第 6 步覆盖。**不要提前用 `gh release create` 建 Release**，否则可能与自动生成的正文互相覆盖。

## 第 6 步：在 Release 页面写更新日志

草稿按仓库约定落到 `.task/release-notes-vX.Y.Z.md`（`.task/` 已在 gitignore 中）。

格式要求（用户明确指定，务必遵守）：

- 三段固定标题：`## ✨ 新特性`、`## 🔧 优化`、`## 🐛 修复`。
- 每项就是**一句简短的话**（「修复了 xxx」「优化了 xxx」），不要长段落、不要罗列实现细节。
- **哪一段没有内容就整段删掉**，不允许只留一个空标题。
- 内容从两次 tag 之间的提交里总结。先看全貌再归类：
  ```bash
  git log --oneline v<PREV>..HEAD
  git show -s --format='%s%n%n%b' <sha>     # 逐个看正文，判断用户可感知的变化
  ```
  归类口径：`feat` → 新特性；面向体验的调整、精简、动画、文案 → 优化；`fix` → 修复。一个提交对应一行，多个提交说同一件事就合并成一行。
- 正文只写这三段，不要额外加「安装包」表格之类的章节（Release 页面本身会列出 Assets）。

写入并核对：

```bash
gh release edit v0.2.2 --notes-file .task/release-notes-vX.Y.Z.md
gh release view v0.2.2 --json tagName,isDraft,assets -q '.tagName, (.assets[].name)'
gh release view v0.2.2 --json body -q .body     # 与本地草稿比对，确认没有被自动正文覆盖
```

- 更新日志用中文，与仓库提交信息保持一致。
- 确认 `isDraft=false`，且 6 个安装包齐全：`Veda-X.Y.Z-macos-arm64.{dmg,app.zip}`、`Veda-X.Y.Z-macos-x64.{dmg,app.zip}`、`Veda-X.Y.Z-windows-x64-setup.exe`、`Veda-X.Y.Z-windows-x64.exe`。
- `gh release edit` 报 404 → 回到「前置检查」里的权限问题。

## 收尾

- 向用户报告：commit、tag、CI run 结论、Release 链接与本版更新日志内容。
- 清掉临时产物（`.task/gocache`、临时 JSON payload 等），使 `git status --short` 保持干净。

## 检查清单

- [ ] `gh` 当前账号有 push/admin 权限（不是只读账号）
- [ ] 版本号来自用户，或已把推断结果与理由告知用户并获同意
- [ ] 打包资源全部同步，无旧版本号残留
- [ ] main 与附注 tag 都已推送成功
- [ ] CI run `conclusion = success`，Release 有 6 个安装包
- [ ] Release 正文为三段式、每项一句话、无空标题
- [ ] 工作区干净，临时文件已清理
