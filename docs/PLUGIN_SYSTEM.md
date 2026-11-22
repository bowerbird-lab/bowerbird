# Bowerbird 插件系统与 API 规范

> 本文档是 Bowerbird 插件系统的**单一权威契约（Single Source of Truth）**，同时约束桌面端 Go 宿主、`@bowerbird/plugin-sdk` 以及外部插件仓库。

---

## 1. 核心架构与职责边界

Bowerbird 桌面端主体**完全采用 Native 实现（Go + `mygo/ui`）**。插件系统的核心原则是 **“App 只提供通用底座与标准协议，半点不碰插件的业务内容”**：

* **App 本体**：负责基础媒体（图片、视频）的原生预览、资源网格（永远为纯图片/占位缩略图渲染）、通用设置与日志、以及插件沙盒宿主。
* **插件包**：完全自维护自身业务，通过以下**两种载体**实现能力：
  1. **隔离 WebView 页面（负责所有自定义 UI 与扩展格式预览）**：运行在 `mygo.WebView`（`Untrusted: true`），支持透明背景（`Transparent: true`）无缝融入原生界面，或独立窗口 / 受控外部网页。
  2. **moejs 沙盒 Hooks（负责无头逻辑与数据提取）**：运行在 Go 进程内嵌的 `moejs` JS/TS 引擎（零构建，支持直接加载 `.js`/`.ts`/`.tsx`），无 DOM 与 Node API。

---

## 2. 插件目录结构

```text
my-plugin/
├── plugin.json          # 唯一清单文件（必填）
├── index.js             # 可选：moejs 无头 Hook 入口（也可用 index.ts）
├── view.html / ui/      # 可选：WebView 预览页或窗口页面
└── bin/                 # 可选：需通过 host.spawn 调用的自带原生 CLI
```

---

## 3. `plugin.json` Manifest 规范

`id`、`name`、`version` 为必填项，且 **`main`、`preview.entry`、`ui`（`windows` / `panels`）至少声明其一**。

```jsonc
{
  "id": "author.my-plugin",          // 仅允许 [a-zA-Z0-9_.-]
  "name": "My Plugin",
  "version": "1.0.0",
  "minAppVersion": "1.0.0",
  "description": "插件描述",
  "author": "Author",
  "license": "MIT",
  "homepage": "https://...",
  "repository": "https://...",
  "platforms": ["desktop"],          // "desktop" | "mobile"

  "kind": "preview",                 // "preview" | "metadata" | "resolver" | "action"
  "priority": 50,                    // 数值越大优先级越高
  "extensions": ["psd", "psb"],      // 小写扩展名，不含前导点
  "mimePrefixes": ["image/vnd.adobe.photoshop"],
  "urlHosts": ["example.com", "*.example.com"],
  "urlPatterns": ["^https?://..."],
  "capabilities": ["preview"],

  "preview": {
    "entry": "view.html",            // 预览区 WebView 入口（相对路径）
    "supportsZoom": true
  },

  "ui": {
    "windows": [
      { "id": "studio", "title": "Studio", "entry": "ui/index.html" },
      { "id": "web", "title": "Web App", "url": "https://example.com/", "interceptDownloads": true }
    ],
    "panels": [
      { "id": "panel", "title": "Panel", "entry": "panel/index.html" }
    ],
    "inspector": [
      { "id": "open-studio", "type": "window", "title": "打开工具", "window": "studio" }
    ]
  },

  "main": "index.js",                // 声明 main 时，hooks 必填且非空
  "hooks": ["resolveUrl", "inspect", "actions", "runAction", "thumbnail"],

  "permissions": {
    "assets": ["read", "write"],     // 仅允许 "read" | "write"
    "network": ["api.example.com"],  // 域名白名单，支持 *.example.com，禁止 bare "*" 与协议/路径
    "process": ["my-cli"],           // 可执行文件白名单，禁止 "*"
    "openExternal": true             // 允许调用 host.platformOpen
  },

  "settings": [
    {
      "key": "apiKey",
      "label": "API Key",
      "help": "说明文字",
      "placeholder": "...",
      "default": "",
      "secret": true
    }
  ]
}
```

### 校验规则
* `ui.windows` 的每一项中 `entry` 与 `url` **互斥**；`url` 必须为合法 `https://` 地址；`ui.panels` 不允许使用外部 `url`。
* `ui.inspector[].window` 必须引用 `ui.windows` 中已存在的 `id`。
* 声明 `main` 时必须声明至少一个 `hooks`；未在 `hooks` 中声明的导出函数不会被宿主执行。

---

## 4. App 提供的通用挂载槽位（Slots）

| 槽位 | 触发条件 | 载体 | 说明 |
| --- | --- | --- | --- |
| **文件预览区** | `preview.entry` + 匹配 `extensions`/`mimePrefixes` | 隔离 WebView | 按 `priority` 选最高优先级插件；开启 `Transparent: true`，可融入原生背景。 |
| **独立窗口** | `ui.windows` | 隔离 WebView | 本地 `entry` 走 `bowerplugin://`；外部 `url` 使用独立持久化 Session，支持 `interceptDownloads` 拦截下载并自动入库。 |
| **详情侧边栏** | `inspect` Hook / `ui.inspector` | 声明式原生渲染 + 按钮 | `inspect` 返回结构化 JSON 由 `mygo/ui` 原生绘制；`ui.inspector` 添加按钮打开指定窗口。 |
| **右键菜单** | `actions` + `runAction` | moejs + 原生菜单 | 宿主为动作 ID 自动加 `<pluginID>:` 前缀。 |
| **URL 解析** | `resolveUrl` + `urlHosts`/`urlPatterns` | moejs | Host 建 O(1) 索引后跑正则，按 `priority` 取第一个非空结果。 |
| **列表缩略图** | `thumbnail` Hook | moejs（纯图片输出） | 列表网格永远渲染静态图片/占位样式；插件可解码冷门格式返回 `imagePath` 给宿主缓存。 |

---

## 5. WebView 宿主协议与 `/_api/*` Bridge

本地打包页面通过 `bowerplugin://localhost/<entry>` 加载：
* **安全隔离**：`Untrusted: true`，临时独立 Session，严格 CSP，通过 `mygo.CallerPage` 绑定请求与窗口页面身份，URL 无法伪造插件身份。
* **权限指纹（`UIGranted`）**：敏感 API 需用户在设置中显式授权，授权指纹绑定 `permissions` 的 SHA-256，权限列表变更时自动失效。

### 可用 Bridge 端点

| 方法与路径 | 所需权限 | 说明 |
| --- | --- | --- |
| `GET /_api/theme` | 无 | 返回宿主当前主题配置 `{ mode, light, dark }`（含语义颜色与圆角），用于让插件页面自动匹配 App 主题。 |
| `GET /_api/settings/{key}` | 无（仅限 `settings` 已声明的 key） | 返回 `{"value": "..."}`。 |
| `POST /_api/network` | `permissions.network` + 用户授权 | 请求体 `{ url, method, headers, bodyBase64 }`。仅限 `https://`，屏蔽 `Host`/`Cookie`/`Content-Length`，无 CookieJar，重定向同样校验白名单；单次上限 `24 MB`，超时 `90s`。 |
| `GET /_api/asset` | `permissions.assets` 含 `"read"` + 用户授权 | 读取打开窗口/预览时绑定的选中资产，上限 `24 MB`，返回 `{ id, name, filename, mimeType, bodyBase64 }`。 |
| `GET /_api/asset/raw` | `permissions.assets` 含 `"read"` + 用户授权 | **流式返回**当前绑定资产的原始文件字节（支持 `Range` 与浏览器直接解码，无 Base64 膨胀）。 |
| `POST /_api/asset/save` | `permissions.assets` 含 `"write"` + 用户授权 | 请求体 `{ mode: "create" \| "replace" \| "variant" \| "page", filename, bodyBase64, prompt? }`，上限 `24 MB`；`page` 会将新图作为新 Page 挂载到当前选中项目（若当前已选子 Page 则自动归属到所属主项目），保存后自动刷新视图与缩略图。 |

---

## 6. moejs 沙盒与 `globalThis.host` API

每次 Hook 调用都在独立 `moejs` Runtime + EventLoop 中执行（编译模块按 `mtime + size` 缓存），**堆上限 `256 MB`、Hook 超时 `60s`、禁用动态 `eval`**。

### 全局能力 (`globalThis.host` & `console`)
* `host.settings.get(key, fallback?)`：读取插件设置（含 `default` 回退）。
* `host.fetch(url, init?)` / `globalThis.fetch(...)`：仅当 `permissions.network` 非空时注入，校验请求与 `Host` 头白名单并走宿主代理。
* `host.spawn(command, args?)`：仅当 `permissions.process` 非空时注入，同步返回 stdout，超时 `20s`。
* `host.platformOpen(url)`：仅当 `permissions.openExternal === true` 时注入，调用系统浏览器打开外链。
* `host.log(level?, message, detail?)` / `console.{log,info,warn,error,debug}`：写入宿主统一日志。

### 5 个标准 Hook 签名
* `resolveUrl({ url })` → `{ url?, candidates?, sourceUrl?, name?, tags?, folderName?, items? } | null`
* `inspect({ item })` → `{ sections: [{ title, rows?: [{ label, value, copyable? }], textBlocks?: [{ title?, text }], actions?: [{ id, title, icon? }] }] } | null`
* `actions({ target, item?, items?, folder? })` → `[{ id, title, icon?, extensions?, disabled? }]`
* `runAction({ id, target?, item?, items?, folder?, ...params })` → `unknown`
* `thumbnail({ filePath, mimeType, item? })` → `{ imagePath, width?, height? } | null`
