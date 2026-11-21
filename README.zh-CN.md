<div align="center">
  <img src="assets/branding/banner.jpg" alt="BowerBird —— 本地优先的素材与设计库管理器" width="100%">
</div>

<h1 align="center">BowerBird</h1>

<p align="center">
  <strong>你的设计素材库，放在你自己的硬盘上。</strong><br>
  写给那些文件太多、耐心太少的人——一个本地优先的素材管理器。
</p>

<p align="center">
  <a href="README.md">English</a> · <strong>简体中文</strong>
</p>

<p align="center">
  <a href="https://awesomensgo.dev/apps/bowerbird"><img src="https://awesomensgo.dev/badge.svg" alt="Listed on Awesome NSGo" height="22"></a>
  <a href="https://awesomemygo.dev/apps/bowerbird"><img src="https://awesomemygo.dev/badge.svg" alt="Listed on Awesome MyGo" height="22"></a>
</p>

---

把 BowerBird 指向一个装着图片和视频的文件夹，它就会变成一个可随手搬走的 `.library` 包：每个文件当场计算 SHA-256 哈希、生成缩略图、提取配色并通过 SQLite 建立索引。无需上传，无需账号，也不经过任何云端。

BowerBird 是一个使用 **mygo（Go）的 GPU 原生 UI（`mygo/ui`）** 构建的跨平台桌面应用，同一工作区内还包含浏览器扩展、插件市场、Android 应用与文档站点。

## 核心特性

- **去重且可移植的素材库** — 每个素材存放在以随机唯一 ID 命名的独立目录（`items/<id>/<filename>`）中，搭配 `index.db`（SQLite，通过 SHA-256 `hex` 索引实现导入去重）与单条目 `metadata.json`；全局设置与已安装插件独立存放在系统用户数据目录中。
- **按画面找图与多维筛选** — 自动提取主色与 6 色感知调色板、ΔE 感知配色筛选、EXIF 元数据解析、虚拟文件夹、标签以及方向/类型筛选。
- **原生预览与画廊交互** — 等比（justified）画廊布局、全画布框选、双击内联预览、空格全屏 QuickLook，以及直接拖出素材到外部应用。
- **可扩展插件体系** — 免构建原生插件（`markdown`、`audio`、`psd`、`prompt`、`video` 与 URL 解析器），并支持插件市场安装与管理。

## 运行与构建

所有开发与构建脚本均位于根目录 `package.json` 中。通过 `scripts/mygo.mjs` 路由的命令会自动强制启用 `CGO_ENABLED=1`（macOS 原生 cgo 实现所必需）。

### 开发运行

```bash
pnpm dev                 # mygo 原生桌面应用开发模式
pnpm dev:marketplace     # 插件市场（apps/marketplace）开发服务器
pnpm dev:docs            # 文档站点（apps/docs）开发服务器
pnpm dev:extension       # 浏览器扩展开发（Chrome；另有 :firefox 变体）
```

### 构建打包

```bash
pnpm build               # mygo 原生应用构建（当前平台）
pnpm build:app:macos     # macOS 通用包（arm64 + amd64）
pnpm build:app:windows   # Windows amd64（便携 exe + NSIS 安装包）
pnpm build:app:linux     # Linux amd64

pnpm build:packages      # 市场/扩展共用的 TypeScript 类型包
pnpm build:extension     # 浏览器扩展（Chrome；另有 :firefox / :edge / :safari）
pnpm build:marketplace   # 插件市场应用
pnpm build:docs          # 文档站点
pnpm build:all           # 所有 JS/TS 工作区目标
```

### 测试与清理

```bash
pnpm test                # go test ./...
pnpm test:watch          # go test -count=1 -v ./...
pnpm typecheck           # 执行所有 JS/TS 包的类型检查
pnpm clean               # 清理构建产物（dist、build/mygo 等）
```

## 相关文档

- [技术架构文档](docs/ARCHITECTURE.zh-CN.md) — 项目结构、原生插件架构、系统级拖拽导出、数据与存储规范、工作区包解析及 NSIS 打包细节。
- [更新日志](CHANGELOG.md) — 各版本变更记录。
