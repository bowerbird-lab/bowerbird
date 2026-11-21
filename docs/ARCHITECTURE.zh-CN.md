<p align="center">
  <a href="ARCHITECTURE.md">English</a> · <strong>简体中文</strong>
</p>

# BowerBird 技术架构文档

本文档详细说明仓库结构、原生插件架构、跨平台拖拽导出实现、数据与存储规范以及构建打包细节。快速上手与常用运行/构建命令请见 [README.zh-CN.md](../README.zh-CN.md)。

## 项目结构

```text
├── main_mygo.go                      # mygo 应用入口（窗口、菜单、托盘、系统级文件拖放路由）
├── mygo.json                         # mygo 构建配置（应用名、bundle id、版本、开发/构建命令）
├── core/                             # Go 后端包
│   ├── models.go                     # 领域模型（Item、ImageMeta、Folder、Tag、LibraryInfo）
│   ├── library.go                    # SQLite 索引、导入流水线、文件夹/标签/回收站增删改查
│   ├── color_extractor.go            # 感知调色板量化器与 EXIF/图片元数据解析
│   ├── asset_server.go               # 为原图、缩略图与系统图标提供本地 HTTP 流式服务
│   ├── service.go                    # 通过 mygo.Bind 暴露给前端的应用服务
│   ├── host.go                       # 桌面运行时抽象（事件、对话框、拖拽、主线程）
│   └── drag_*.go                     # 各平台的系统级拖拽导出与调试追踪
├── internal/
│   ├── app/                          # mygo/ui 原生视图与主线程状态机
│   │   ├── ui_main.go                # 工作区外壳、路由与快捷键
│   │   ├── ui_gallery.go             # 等比画廊、选择与系统级拖拽
│   │   ├── ui_inspector.go           # 原生检查器与插件扩展区块
│   │   ├── ui_preview.go             # 图片、Markdown、音频、视频与 PSD 预览
│   │   └── ui_settings.go            # 设置、插件管理与数据目录
│   ├── plugin/                       # 原生插件宿主（清单、沙箱、内置插件）
│   ├── player/                       # Go 音频播放内核
│   └── i18n/                         # 中英文词典
├── plugins/                          # 免构建官方插件
│   ├── markdown-preview/             # plugin.json + inspect 钩子
│   ├── audio-player/
│   ├── psd-preview/
│   ├── prompt-metadata/
│   └── pinterest/                    # 沙箱 URL 解析器
├── packages/plugin-sdk/              # 原生插件 TypeScript 类型定义
└── packages/ui/                      # 浏览器扩展共用的 React 组件库
```

---

## 原生插件架构

桌面端由 `mygo/ui`（Go + GPU）原生渲染，不再包含 WebView、React 运行时或插件 CSS。插件只返回**纯 JSON**，由宿主用原生视图绘制。

### 官方插件

| 插件 ID | 版本 | 类型 | 原生渲染器 / 能力 |
| :--- | :--- | :--- | :--- |
| `official.markdown-preview` | `2.0.0` | preview | `markdown` — GFM 渲染、Frontmatter 与大纲检视 |
| `official.audio-player` | `2.0.0` | preview | `audio` — 播放、频谱、进度条与音频元数据 |
| `official.psd-preview` | `2.0.0` | preview | `psd` — 合成图解码、缩放与图层树 |
| `official.prompt-metadata` | `2.0.0` | metadata | `prompt` — AI 提示词字段、复制与全文编辑 |
| `builtin.video` | `2.0.0` | preview | `video` — ffmpeg 解码帧 + `ui.Video`，mygo 音频输出 |
| `official.pinterest` | `2.2.0` | resolver | moejs 嵌入式运行时中的 `resolveUrl` |

源码位于 `plugins/<plugin-id>/`（`plugin.json`，仅 resolver 插件保留 `index.js`），无需任何构建步骤。打包版本会把相同清单注册为编译进二进制的内置插件；本地目录可覆盖内置副本。

### 插件清单

```json
{
  "id": "official.markdown-preview",
  "name": "Markdown Preview",
  "version": "2.0.0",
  "kind": "preview",
  "native": "markdown",
  "extensions": ["md", "markdown"],
  "mimePrefixes": ["text/markdown"],
  "priority": 50
}
```

原生预览插件可以省略 `main`。`resolveUrl`、`inspect`、`actions`、`runAction`、`thumbnail` 等 JS 钩子均为可选，但必须在 `hooks` 中显式注册；宿主不会调用未声明的钩子。它们运行在 **moejs** 嵌入式运行时中，并受明确的网络/进程/外链权限、内存上限和超时限制。每次 hook 开始/结束、返回值、异常以及宿主 API 调用都会写入原生插件日志；插件可通过 `host.log()` 或 `console.*` 输出自己的日志。TypeScript 类型定义位于 `packages/plugin-sdk`，完整指南见 `plugins/README.md`。

---

## 拖拽导出到窗口之外

把素材拖到别的应用是平台相关的，因为从 webview 发起的 HTML5 拖拽只会发布文本格式（`text/plain`、`text/uri-list`）：

- **Windows**（`core/drag_windows.go`）：`frontend/src/lib/drag-out.ts` 取消 webview 的拖拽，并以被拖条目 ID 调用 `Service.StartDragItems`。Go 侧构造 shell 数据对象（`SHCreateDataObject` → `CF_HDROP`），并以 `DROPEFFECT_COPY` 运行 `DoDragDrop`。
- **macOS**（`core/drag_darwin.go`）：保留 webview 的拖拽会话，并把其剪贴板改写成 `public.file-url` / `NSFilenamesPboardType`。
- **应用内拖放视觉**：侧边栏与主工作区都使用不打扰操作的 `10%` 不透明度遮罩加虚线边框（`bg-foreground/10 border-dashed`），既不遮挡文件夹名，也不引起布局位移。

### 拖放追踪

拖放流水线的每一步都会写入用户数据目录下的 `drag-debug.log`（Windows 为 `%APPDATA%\bowerbird\drag-debug.log`，macOS 为 `~/Library/Application Support/bowerbird/drag-debug.log`）。测试期间可通过 `BOWERBIRD_USER_DATA_DIR` 覆盖该目录。

---

## 数据与存储架构规范

BowerBird 在 **用户素材库** 与 **应用状态数据** 之间执行严格的物理隔离：

```text
┌────────────────────────────────────────────────────────┐
│ 1. 用户素材库（可移植容器包）                           │
│    例如 ~/Pictures/Designs.library/                    │
│    ├── items/<id>/<filename>   （随机大写字母数字 ID）   │
│    ├── items/<id>/thumbnail.jpg                        │
│    ├── items/<id>/metadata.json                        │
│    └── index.db（SQLite 缓存 + SHA-256 hex 去重索引）   │
└────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────┐
│ 2. 应用用户数据目录（<UserDataDir>）                    │
│    macOS:   ~/Library/Application Support/bowerbird/   │
│    Windows: %APPDATA%\bowerbird\                       │
│    Linux:   ~/.config/bowerbird/                       │
│    ├── user_data.json      # 全局界面设置               │
│    ├── app.log             # 诊断运行时日志             │
│    ├── drag-debug.log      # 拖放追踪日志               │
│    └── plugins/            # 已安装的远程插件           │
│        ├── installed.json  # 已安装插件注册表           │
│        └── <plugin-id>/    # 解压后的插件包             │
│            ├── manifest.json                           │
│            └── index.js                                │
└────────────────────────────────────────────────────────┘
```

### 插件生态与分发规范

1. **原生源码布局**（`bowerbird-lab/plugins` 的 `main` 分支）：
   - 官方插件位于 `plugins/<plugin-id>/`，由 `plugin.json` 和可选的免构建 `index.js` 组成。
   - 预览插件声明宿主渲染器（如 `"native": "markdown"`），不再包含 React 组件，也不需要 Vite/Rollup 构建产物。
2. **双通道分发仓库**：
   - 构建产物由 `bowerbird-lab/plugins` 自动发布到孤儿分支 `repo` 与 GitHub Releases。
   - **`repo` 孤儿分支**：精简索引元数据（`index.min.json`、`registry.json`）与 `repo/<id>/plugin.json`。
   - **GitHub Releases**：打包好的发布 zip（`<plugin-id>-v<version>.zip`）。
3. **市场与边缘解析**：
   - 插件商店读取 `PLUGIN_REPO_INDEX_URL`，发布包直接从 `PLUGIN_RELEASE_BASE_URL` 下载，不经过代理。
4. **运行时状态**：
   - `<UserDataDir>/plugins/installed.json` 为旧 WebView 插件注册表；原生宿主使用 `native-plugins.json` 保存启停与设置。
   - 设置 → 数据与存储可直接打开插件目录与应用数据目录。

---

## 构建与打包细节

### cgo 要求

`core/` 下的 macOS 原生实现（`sysicon_darwin.go`、`videothumb_darwin.go`、`drag_darwin.go`）包含 `import "C"`。设置 `CGO_ENABLED=0` 时 Go 会静默丢弃它们，而它们的 `!darwin || ios` 兜底实现同样会在 darwin 上被排除——于是 `StartDragItems` / `GetSystemFileIconPNG` 等符号消失，构建以 `undefined: ...` 报错失败。`pnpm dev` / `pnpm build:app*` 脚本都经由 `scripts/mygo.mjs` 路由，会在所有平台上强制 `CGO_ENABLED=1`。

### 工作区包解析

`@bowerbird/plugin-sdk` 与 `@workspace/ui` 直接从 TypeScript 源码消费——`@bowerbird/plugin-sdk` 发布的 `dist/` 产物只由 `pnpm --filter @bowerbird/plugin-sdk build` 生成，且只有发布到 npm 时才需要。

- **类型检查**——在消费方的 `tsconfig.json` `paths` 中映射该包（替换项相对 `baseUrl` 解析）：
  `"@bowerbird/plugin-sdk": ["../packages/plugin-sdk/src/index.ts"]`。
- **Vite**——设置 `resolve.tsconfigPaths: true`，让开发期解析与 `tsc` 一致（`frontend/vite.config.ts`）；生产构建无论如何都会内联源码。

缺少 Vite 配置时，开发会失败并报 `Failed to resolve import "@bowerbird/plugin-sdk" from ... Does the file exist?`。

### Windows 安装包（NSIS）

在 Windows 上，`go tool mygo build`（经由 `pnpm run build:app:windows`）会同时产出便携版可执行文件与位于 `build/mygo/windows-amd64/` 下的 NSIS 安装包。安装包会在 *应用和功能* 中注册该应用，在缺少 WebView2 运行时时自动安装，并询问用户是否创建快捷方式（默认勾选开始菜单，桌面可选），以及是否运行应用。

NSIS 步骤是 mygo Windows 打包流水线的一部分；GitHub 的 `windows-latest` runner 开箱即可构建。本地 Windows 构建时，需保证 `makensis` 在 `PATH` 中：用 `scoop install nsis`（shim 会自动进入 `PATH`）、`choco install nsis`（把 `C:\Program Files (x86)\NSIS` 追加到 `PATH`），或 `winget install NSIS.NSIS`（需手动配置 `PATH`）。

```bash
go tool mygo build
# -> build/mygo/windows-amd64/   便携可执行文件 + NSIS 安装包
```

CI（`.github/workflows/release-app.yml`）会以 `bowerbird-windows-x64-portable.exe` 与 `bowerbird-windows-x64-installer.exe` 两个名字发布它们。
