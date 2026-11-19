# Bowerbird 通用插件 UI 与能力边界方案

## 目标与范围

App 负责素材库、移动端同步、通用插件运行环境和用户授权；插件负责各自的业务逻辑与界面。Mage 是外部插件，不在 App 中维护 Mage 专用窗口、参数、模型列表、上传或轮询逻辑。

第一阶段以桌面端 WebView 插件界面为主，保留现有原生插件和 JSON hook 的兼容性。Native 插件 UI 作为后续可选方案，不要求插件作者申请 UI 权限。移动端插件 UI 暂不承诺支持，需独立评估 NSGo 和平台能力。

## 当前情况

- `packages/plugin-sdk/src/types.ts` 目前只允许插件返回数据，交由 App 原生渲染；插件没有自己的交互界面。
- `internal/plugin` 使用 moejs 运行插件 hook，`host.fetch` 使用 manifest 域名限制；现有 schema 还有 process 和 openExternal。
- mygo 支持 WebView，但项目当前没有通用的插件 WebView 容器、隔离和双向桥接。需先以 macOS、Windows、Linux 的实际能力验证安全边界，不能仅凭 API 名称假设各平台都能拦截所有网络请求。
- PR #67 包含 Mage 专用宿主界面、素材版本能力以及移动端改动；它们需要分开评审。

## 插件界面

插件包内提供 HTML、CSS 和 JavaScript；App 支持两种通用入口：

1. 检查器面板：用户选择素材后展示插件贡献的界面，插件通过事件获取当前选择变化。
2. 独立窗口：用户从插件入口或面板打开；窗口由宿主管理生命周期，内容由插件实现。

manifest 可声明贡献位置和本地入口，不把 AI、Mage、模型或参数写入 App。示意（具体字段以 SDK 实现为准）：

```json
{
  "id": "official.mage-ai-image",
  "name": "Mage AI Image",
  "version": "2.0.0",
  "ui": {
    "windows": [{ "id": "studio", "title": "Mage Studio", "entry": "ui/studio.html" }],
    "sidebar": [{ "id": "tools", "type": "group", "title": "Mage AI", "children": [
      { "id": "editor", "type": "window", "title": "图片编辑", "window": "studio" }
    ] }]
  },
  "permissions": {
    "assets": ["read", "write"],
    "metadata": ["read", "write"],
    "network": ["api.mage.space", "*.mage.space", "storage.googleapis.com"]
  }
}
```

权限仅作示例，插件应只声明实际使用的能力；UI 入口文件和静态资源必须位于插件目录中。

侧边栏使用 `ui.sidebar` 声明：`group` 节点可嵌套 `children`，`window` 节点通过 `window` 引用同一个插件在 `ui.windows` 中声明的窗口 ID。插件禁用后隐藏入口；`type` 不合法、重复 ID 或引用不存在的窗口会拒绝该清单。目前侧边栏入口打开独立 WebView 窗口，检查器内嵌面板尚未实现。

## 权限与用户体验

以少量能力组呈现，安装时展示申请的权限与网络域名，用户可在设置中撤销；运行时每次调用均校验插件身份、当前授权及目标范围。无权限则返回清楚的可处理错误。

| 能力 | 说明 |
| --- | --- |
| 素材读取 | 读取获授权素材的文件内容。默认以用户选择或显式交给插件的素材为范围；全库访问需明确展示与单独授权。 |
| 素材写入 | 新增、替换或删除素材；操作按素材 ID 经宿主执行。插件负责操作界面，App 校验授权并统一处理素材变更。 |
| 元数据读取 | 获取获授权素材的名称、标签、来源、提示词及其他元数据；元数据同样可能敏感。 |
| 元数据写入 | 修改授权范围内的素材元数据；宿主负责校验与一致性。 |
| 网络 | 声明需要访问的域名，默认无网络；展示域名和联网可能上传数据的说明，重定向后仍逐跳检查。 |

UI 窗口、组件和插件自有界面状态不需要额外授权。API 不向页面直接暴露任意系统文件路径或宿主进程的完整权限。素材读取与网络组合后可上传数据，因此授权展示需明确指出这两种权限同时存在时的含义。

现有 `process` 与 `openExternal` 的兼容性需要单独处理：保留老插件行为，迁移期不自动扩大权限；新模型不要求一般 UI 能力声明权限。进程执行可以绕过文件和网络边界，不能无条件降为普通 UI 能力，应作为高级危险能力单独评估；外部链接通过宿主处理，阻止危险协议。

权限变更（包括升级新增域名或扩大文件范围）需重新展示并获得授权；拒绝不影响已有权限。不要把 manifest 声明本身当作用户授权。插件禁用或卸载后撤销访问，关闭其窗口并终止请求。

## WebView 隔离与资源加载

- 每个插件使用隔离的来源和存储空间；页面只能加载其插件包中允许的资源，阻止目录穿越、符号链接逃逸和其他插件的资源访问。优先采用平台支持的受控自定义 scheme / 虚拟来源；具体机制以各平台 PoC 为准。
- 默认禁止页面直接访问外网、任意 `file:` 路径和宿主本地服务；导航、新窗口、WebSocket、表单、图片、CSS、iframe、重定向等所有出口均须纳入验证。远程代码和远程页面不获得桥接权限。
- 网络请求经宿主桥接，逐跳校验域名、协议及当前授权，使用 App 代理设置；不能只依赖前端 CSP。输出中不记录 API key、Authorization、完整签名 URL 或敏感请求体。
- 桥接绑定到插件 ID 和可信页面实例，校验来源、消息结构及关联请求；不能让其他页面伪造插件身份。不同插件之间不共享桥接对象或设置。
- CSP 等浏览器限制属于纵深防御，实际拦截能力和不同平台差异须通过攻击性测试验证；若某平台不能可靠限制，则该平台不开放不安全的插件 WebView，先补平台实现。
- 插件设置按插件隔离；用户可查看和编辑自己电脑上的密钥。敏感字段默认遮罩并提供显示/隐藏按钮，不在日志中输出；不宣称已使用系统密钥链。

## 插件配置与密钥槽

插件通过 manifest 声明多个配置项，App 自动生成设置界面；插件通过 API 读取自己的值，无需为每个密钥单独实现宿主界面。`key` 在插件内唯一，支持名称、说明、占位文字、默认值、是否必填及类型约束。示意：

```json
"settings": [
  { "key": "apiKey", "label": "Mage API Key", "type": "string", "secret": true, "required": true },
  { "key": "endpoint", "label": "API 地址", "type": "url", "default": "https://api.mage.space" },
  { "key": "timeout", "label": "超时（秒）", "type": "number", "default": 60, "min": 1 },
  { "key": "quality", "label": "画质", "type": "select", "options": [{ "value": "standard", "label": "标准" }, { "value": "high", "label": "高" }] },
  { "key": "enabled", "label": "启用功能", "type": "boolean", "default": true }
]
```

初版支持 `string`、`multiline`、`number`（可声明整数、范围和步长）、`boolean`、`select`、`url`；可选增加 `multi-select`。不预设插件必须使用所有类型。`secret` 是独立标记，适用于密钥、令牌等字符串：默认遮罩、用户可切换为明文查看，复制和修改由用户控制。用户在自己设备上有权查看，插件通过 `settings.get(key)` 读取自己的值；不允许跨插件读取。日志、错误和诊断导出不应包含密钥。配置本地存储的具体保护方式和卸载时保留/删除选择需明确说明，不把遮罩等同加密。

插件可通过 `settings.open()` 跳转到自己的配置页；保存后应通知已打开的插件界面，便于重新加载模型。升级插件时按插件 ID 和字段 key 保留已有值，兼容现有 `settings` 清单与 `native-plugins.json`。配置可定义多个密钥，无需对每个槽单独弹窗授权。

## SDK 与宿主 API 草案

WebView 内通过 Promise 异步桥接调用；返回结构化错误（权限拒绝、资源不存在、网络错误等），不要吞掉插件异常。以下为概念接口，API 命名以 PoC 后的 SDK 为准：

```ts
const items = await bowerbird.selection.current();
const image = await bowerbird.assets.read(items[0].id);
const meta = await bowerbird.metadata.get(items[0].id);
const key = await bowerbird.settings.get("apiKey");
const response = await bowerbird.network.fetch("https://api.mage.space/v1/...", {
  headers: { Authorization: `Bearer ${key}` }
});
await bowerbird.assets.create({ name: "result.png", data: result });
await bowerbird.assets.replace(items[0].id, { data: result });
await bowerbird.metadata.update(items[0].id, { tags: ["edited"] });
await bowerbird.windows.open("studio");
```

- 大文件使用流或受限的临时句柄，避免把全部二进制通过 JSON/base64 传输；预览资源使用有作用域和有效期的句柄。
- 提供选择变化、任务取消、窗口关闭等事件；卸载或撤权时中断未完成操作。
- `network.fetch` 由 App 代发，处理 CORS、代理、授权域名和响应；需要支持 JSON、二进制上传/下载、取消、超时、状态码和可读错误，不能吞掉 Mage 402 等错误。
- 插件只决定何时创建或替换并提交结果文件；App 沿现有导入/更新流程统一完成文件持久化、数据库索引、缩略图、尺寸与元数据提取、颜色/调色板生成、缓存刷新、同步及界面通知。失败时返回明确结果，不让插件自行维护这些内部状态。
- 替换复用宿主统一的版本和恢复策略；插件负责展示自己的操作界面，不为每个插件在 App 硬编码确认对话框。具体历史保留与删除规则由通用素材服务定义并向用户说明。
- 保存为新素材、替换并保留旧版本、列出和恢复版本是通用素材服务；结果处理不出现 Mage 字段。
- 插件可使用自己的设置与日志；插件错误保留可读 message/code，敏感内容不进入日志。
- SDK 提供类型、简单示例、权限描述、插件打包规则和运行时版本声明。

## Mage 迁移

1. 在 `bowerbird-lab/plugins` 中实现插件自有 HTML 检查器面板和工作台窗口，业务逻辑负责模型列表、参数、上传、任务状态和错误展示。
2. 通过通用素材 API 获取用户选择并读取图片；读取自己的 API key，通过 App 代发网络请求；结果通过素材 API 新增或替换，余下导入、索引与颜色等处理由 App 完成。
3. App 移除 `ui_ai_editor.go` 中 Mage 专用状态、模型默认值和工作流，以及专属工具栏/窗口入口；通过插件贡献注册通用入口。检查器的通用版本列表可独立保留。
4. Mage 插件的现有设置与 API key 应迁移或兼容，更新前先验证，避免丢失用户配置。
5. 迁移完成后测试无 Mage 插件、插件禁用、拒绝网络和撤销文件权限等情况。不要在桥接未完成时删除现有可试用实现。

## 实施顺序与 PR 拆分

1. **拆分现有 PR**：暂停合并 #67；将移动端功能和通用素材版本从 Mage 专用实现中分开。先保留原分支与本地测试改动，避免丢失；分别说明各 PR 依赖和测试状态。通用密钥输入修复可独立提交。
2. **跨平台 PoC**：验证 WebView 可嵌入检查器及独立窗口、加载插件内资源、桥接、来源隔离、网络和文件出口控制。产出平台支持矩阵；未验证的平台不得标记为安全可用。
3. **权限与通用宿主**：统一清单与授权记录，增加素材、元数据、网络 API；兼容现有插件和授权，不静默扩权。
4. **UI 容器与 SDK**：增加插件贡献发现、面板/窗口生命周期、类型定义、示例、错误处理和打包验证。
5. **Mage 插件迁移**：在插件仓库实现并发布新版本，在 App 中删除专用实现；保留通用素材版本能力。
6. **Native 后续**：根据真实插件场景再评估声明式原生 UI 或其他方案，不把 WebView 插件强制迁移。

## WebView 实施记录

当前工作分支已将独立窗口接入非受信任 WebView，页面由 `bowerplugin` 协议按宿主提供的页面身份加载资源，并使用临时隔离会话。设置页对已启用且声明窗口的插件显示入口；禁用后后续资源请求被拒绝。插件目录使用 Go `os.Root` 限定并拒绝目录遍历和越界符号链接，测试覆盖隔离、撤销和路径逃逸。`examples/plugins/webview-demo` 是静态界面示例。此阶段尚无素材、设置、网络桥接，实际桌面加载及跨平台安全边界仍需验证。

mygo v0.9.2 的 `PageOptions.Untrusted` 禁止绑定 Go 方法；`Protocol.Handle` 可通过 `CallerPage` 识别页面，可用于下一阶段设计桥接。`ContentRules` 文档明确指出 Windows 上 WebSocket 不受该机制拦截，不能把 content rules 当成完整网络隔离；启用前必须结合 CSP 和平台实测，任何不能可靠约束网络的路径都不开放。

## 验收

- 无需修改 App 即可安装新的 HTML 插件并在检查器或独立窗口显示；卸载和禁用后入口消失。
- 验证多个配置槽及其输入类型、密钥显示/隐藏、设置变更通知、旧配置升级保留；密钥不会进入日志。
- 素材新增和替换复用 App 流程：正确生成或刷新缩略图、颜色、索引、缓存与同步状态，失败不会留下不一致的数据。
- Mage 可配置密钥、选择模型、读取选中图片、生成、显示错误与结果、保存素材；App 无 Mage 专用代码。
- 未授权读取、写入、元数据和网络都被拒绝；展示的域名与实际允许的一致，重定向、WebSocket、图片和 iframe 等无法绕过限制。
- 插件无法访问其他插件资源、任意文件、本地服务或冒充其他插件桥接身份；撤权后立即生效。
- 原有 resolver、preview、metadata、action 插件行为与设置兼容；权限升级不会静默授权。
- 各桌面平台分别验证实际 WebView 能力和安装依赖；现有核心、插件及移动端测试通过或明确列出尚未解决的问题。
