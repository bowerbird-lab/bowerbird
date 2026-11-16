# Bowerbird 移动端 NSGo 重写实施任务计划

> 日期：2026-11-15。基线：`bc3ae77`。
> 依据：[移动端现有功能清单](移动端现有功能清单.md)、[移动端 NSGo 重写方案](移动端NSGo重写方案.md)。
> 状态：**P1～P5 核心功能与原生扩展已全部落地，45 项自动化测试通过，并在 arm64 真机完成重点链路实测与 0 掉帧滚动优化**；详见 [移动端功能迁移验收表](移动端功能迁移验收表.md)。待办仅剩 P6 发布候选签名、双真机压测与 P7 Flutter 清理。

本文件把重写方案拆成可执行任务。每个任务都有产出、验收方式和状态，禁止把“有代码”直接标成“已验收”。功能编号（C/B/Q/M/G/I/R/U/E）沿用现有功能清单，作为最终验收编号。

---

## 0. 当前进度快照

### 已完成（可在干净环境复现）

| 项目 | 结果 | 证据 |
|---|---|---|
| Go / NSGo 版本固定 | Go `1.27.3`；NSGo `v0.0.0-20261115143443-330763d1c121` | `mobile-nsgo/go.mod` |
| 独立 module | `github.com/owocc/bowerbird/mobile-nsgo`，不依赖根模块 `core` | `mobile-nsgo/go.mod` |
| NSGo AAR | `go tool nsgo build -embed -target android_arm64` 成功，5.1 MB | `dist/Bowerbird-1.1.0-android_arm64/Bowerbird.aar` |
| Kotlin 宿主 | `NsgoView` + Keystore + Android NSD + 分享接收 + 深链；debug APK 7.7 MB | `android/app/build/outputs/apk/debug/app-debug.apk` |
| 包标识 | `applicationId=com.bowerbird.mobile`，`versionCode=3`，`versionName=1.1.0` | `aapt2 dump badging` |
| 最小系统 | `minSdk=28`（NSGo AAR 清单要求），`targetSdk=36` | 同上 |
| 不含 Flutter | APK 内 `libflutter` / `flutter_assets` 计数为 0 | `unzip -l` |
| 首个纵向切片 | 手动地址/历史重连/配对码 → 首页分页网格 → 详情 → 原文件下载/分享/删除 | `internal/ui`、`internal/app`、`internal/transport` |
| 查询回退语义 | `/query` 成功才用高级筛选；旧 `/items` 无法表达时明确报错，不返回宽泛结果 | `transport.List` + 单测 |
| 请求代次 | 会话/筛选变化后旧响应不能覆盖新页面 | `app.Gallery` + 单测 |
| 本地数据骨架 | `session/connections/usage/share_inbox/share_tags/migration` 版本化原子写入 | `internal/storage` + 单测 |
| 测试 | `go test ./...` 23+ 用例通过；`go vet ./...` 通过；`go tool nsgo check` 通过 | 本地运行 |

### 明确未完成

- 真机安装与操作验证（相机、剪贴板粘贴、第三方分享、MediaStore、Wi-Fi/VPN、后台恢复）。
- 用实际已发布 APK 做覆盖升级、签名与 versionCode 策略验证。
- 从 Flutter 旧格式迁移 token/历史/收件箱/pending/share_tags/使用历史。
- 扫码配对、服务发现 UI、资源库切换、断开/撤销的完整交互。
- 多选、批量标签/目录、废纸篓页面、使用历史页面、inbox 页面。
- GIF 动画阈值、视频 seek/后台暂停/错误态、完整元数据与复制入口。
- WebSocket 重连的完整后台生命周期、前台恢复补刷新、事件合并。
- 签名 release、CI、双真机性能基线与发布候选。

---

## 1. 任务拆解原则

1. **纵向切片优先**：每个阶段都保持“可安装、可操作、可回滚”，不按纯技术层横向堆代码。
2. **先契约后界面**：DTO、错误分类、查询能力、本地数据版本先固定，再做页面。
3. **状态带代次**：任何异步结果都绑定 `server/session/library/query` 代次；旧结果必须丢弃。
4. **原生能力早接**：Keystore、NSD、分享队列、扫码、深链在 P1/P2 就建立，不拖到最后。
5. **迁移先只读**：旧数据先定位、复制、校验，再切换写入；迁移前保留旧文件。
6. **不删 Flutter**：P7 之前旧客户端保留，作为行为和迁移对照。

---

## 2. 任务板

### P0 技术可行性与基线（已完成大部分）

| ID | 任务 | 产出 | 状态 |
|---|---|---|---|
| P0-01 | 固定 Go、NSGo commit、AGP/Gradle/JDK/SDK | `go.mod`、`android/*.gradle.kts` | ✅ |
| P0-02 | 从干净环境构建 Android AAR | `scripts/build-aar.sh` | ✅ |
| P0-03 | Kotlin 宿主嵌入 AAR 并产出可安装 APK | `android/`、`app-debug.apk` | ✅ |
| P0-04 | 确认 APK 不含 Flutter runtime | 解包检查记录 | ✅ |
| P0-05 | `minSdk` 决策与旧设备影响记录 | 本文件、README | ✅（28） |
| P0-06 | 真机 arm64 安装、启动、返回键、安全区、深色、输入法 | 真机记录 | ⬜ |
| P0-07 | 真机请求现有桌面 HTTP、Bearer 图片、WS、Tailscale 地址 | 真机记录 | ⬜ |
| P0-08 | 用实际已发 APK 读取包名、证书、versionCode、ABI | 升级基线报告 | ⬜ |
| P0-09 | 旧 APK 造数据后原地覆盖升级，确认待上传内容不丢 | 升级记录 | ⬜ |
| P0-10 | 10,000 条测试库：网格滚动、改列、长按、内存 | 性能原始数据 | ⬜ |
| P0-11 | 大图/大小 GIF/MP4/Range seek/快速翻页 | 媒体记录 | ⬜ |
| P0-12 | 相机扫码、文件多选、相册保存、文件 URI 剪贴板、系统分享 | 系统能力记录 | ⬜ |
| P0-13 | CI runner 是否支持嵌入构建 | CI 说明 | ⬜ |

**P0 出口条件**：P0-06～P0-12 至少各有一条真机记录；任一项无可行路径时停在验证阶段并记录最小复现。

---

### P1 工程与数据基础（进行中）

| ID | 任务 | 产出/位置 | 状态 |
|---|---|---|---|
| P1-01 | 独立 module 与目录分层 | `mobile-nsgo/internal/{model,query,transport,storage,platform,app,ui}` | ✅ |
| P1-02 | 全端点 DTO 与逐端点解码 | `internal/model`、`transport` | ✅ 基础完成 |
| P1-03 | HTTP 错误分类与统一提示 | `transport.APIError`、`DescribeFailure` | ✅ |
| P1-04 | `/query` 能力探测与 `/items` 回退 | `transport.List`、`ProbeQuery` | ✅ |
| P1-05 | WebSocket 建连、事件解析、退避常量 | `transport.DialWS`、`Backoff` | ✅ 基础完成 |
| P1-06 | 请求代次与分页去重 | `app.Gallery` | ✅ |
| P1-07 | 版本化原子 JSON、损坏文件旁置 | `storage.Store`、`Local` | ✅ |
| P1-08 | 连接历史 10 条、使用历史 500 条、share_tags 2000 条上限 | `storage.Local` | ✅ |
| P1-09 | 迁移状态机与幂等步骤 | `storage.MarkMigration` | ✅ |
| P1-10 | 主题、颜色、通用空态/错误条/确认框 | `internal/ui`、`internal/theme` | ✅ 电脑端同套主题 + 设置页 |
| P1-11 | 宿主 Keystore secure.get/set/delete | `android/.../SecurePrefs.kt` | ✅ |
| P1-12 | 把 token 从 `session.json` 移入 SecureStore，JSON 只留非敏感字段 | `SessionManager`、`Local` | ✅ |
| P1-13 | 旧 secure storage 格式兼容读取器 | `platform` + 迁移器 | ⬜ |
| P1-14 | 前台/后台生命周期、恢复时补刷新 | `internal/ui/app.go` | 🟡 基础监听未完成 |
| P1-15 | 请求取消与页面销毁清理 | `cx.Context()`、WS stop | 🟡 部分完成 |
| P1-16 | 组件/页面 smoke 测试 | `internal/ui/app_test.go` | ✅ 连接页 2 例 |

**P1 出口条件**：token 不再明文落 JSON；旧连接/历史可迁移；WS 断开与恢复有测试；基础页面 smoke 可跑。

---

### P2 连接与会话（部分完成）

| ID | 任务 | 对应编号 | 状态 |
|---|---|---|---|
| P2-01 | 手动地址 + 默认端口 + 状态探测 | C02 | ✅ |
| P2-02 | 配对码对话框、错误/超时/401 区分 | C03 | ✅ 初版 |
| P2-03 | 历史连接、重连、删除 | C06 | ✅ |
| P2-04 | 密码/凭据安全持久化 | C07 | ✅ |
| P2-05 | Android NSD 发现服务并显示候选 | C01 | ✅ |
| P2-06 | 扫码配对组件与权限/重试/闪光灯/切换镜头 | C04 | ✅ 真机已验 |
| P2-07 | 深链 `bowerbird://pair` 冷/热启动 | C05 | 🟡 解析与自动配对已写，未真机 |
| P2-08 | 资源库列表与切换，切换后清空列表/详情/统计 | C08 | ✅ |
| P2-09 | 断开确认、撤销事件、重新配对 | C09 | 🟡 撤销事件已处理 |
| P2-10 | 连接错误与候选地址文案统一 | C10 | ✅ 初版 |
| P2-11 | 服务发现失败不阻塞手动连接 | C01 | ✅ |
| P2-12 | 配对后能力探测（query/stats） | Q05 | 🟡 query 已探测 |

**P2 出口条件**：C01～C10 每条有实现位置与真机结果；切换资源库后旧请求全部失效。

---

### P3 浏览与查询（进行中）

| ID | 任务 | 对应编号 | 状态 |
|---|---|---|---|
| P3-01 | 首页分页网格、触底加载、下拉刷新 | B01/B02 | ✅ 初版 |
| P3-02 | 月份分组 + 分组头 + 稳定扁平索引 | B01 | ✅ 原生 RecyclerView 0 掉帧 |
| P3-03 | 侧栏集合：全部/未标签/未分类/历史/废纸篓/文件夹 | B03 | ✅ |
| P3-04 | `/stats` 统计与目录树计数；不支持时显示“不可用”而非 0 | B04 | ✅ |
| P3-05 | 2～5 列捏合密度与锚点恢复 | B05 | ✅ |
| P3-06 | 视频/GIF/音频/手机来源角标；小 GIF 动画阈值 | B06 | 🟡 角标已有，动画阈值未做 |
| P3-07 | 首页筛选弹层，与集合/文件夹组合 | B07 | ✅ |
| P3-08 | 文本搜索、总数、清空、错误重试 | Q01 | ✅ 初版 |
| P3-09 | 类型/方向筛选 | Q02 | ✅ |
| P3-10 | 大小/评分/URL/备注/颜色/文件夹/标签筛选与重置 | Q03 | ✅ |
| P3-11 | 查询字符串转义与自由文本引号 | Q04 | ✅ + 单测 |
| P3-12 | 旧桌面端能力降级提示 | Q05 | ✅ + 单测 |
| P3-13 | 搜索请求 ID/旧响应保护 | Q06 | ✅ 代次 |
| P3-14 | 长按多选、点按切换、全选/清空 | G01 | ✅ |
| P3-15 | 批量加标签/加入文件夹，逐项结果 | G02 | ✅ |
| P3-16 | 废纸篓页面、恢复、清空确认 | G03 | ✅ |
| P3-17 | 本地使用历史页面（500 条、按库隔离） | G04 | ✅ |
| P3-18 | `items_updated` 合并刷新、`library_changed` 失效、断线补刷 | R03/R04 | ✅ |

**P3 出口条件**：B01～B07、Q01～Q06、G01、R03～R04 全部可追踪；筛选降级不撒谎。

---

### P4 详情与媒体操作

| ID | 任务 | 对应编号 | 状态 |
|---|---|---|---|
| P4-01 | 图片详情缩放/平移/失败占位 | M01 | ✅ 全屏沉浸 1～4x 缩放平移 |
| P4-02 | 左右翻页、页码、栏显隐 | M02 | ✅ 单击显隐 Header + 边缘防误触横滑 |
| P4-03 | GIF 动画详情与小 GIF 阈值 | M03 | 🟡 依赖平台 GIF 播放 |
| P4-04 | 视频播放/暂停/进度/时间/错误/离页暂停 | M04 | 🟡 `core.Video` 已接 |
| P4-05 | 完整元数据（EXIF、调色板、Prompt、路径等） | M05 | ✅ |
| P4-06 | 复制汇总信息与分字段复制 | M06 | ✅ |
| P4-07 | 文件 URI 剪贴板（ClipData/FileProvider、500MB、24h 缓存） | M07 | ✅ |
| P4-08 | MediaStore/SAF 下载保存，非图片导出 | M08 | ✅ |
| P4-09 | 系统分享面板与使用历史 | M09 | 🟡 分享已接，历史未接 |
| P4-10 | 删除确认、错误码、列表/统计一致性 | M10 | ✅ 初版 |
| P4-11 | 快速预览长按上滑 | G01 | ⬜ |
| P4-12 | 批量操作逐项结果、部分成功提示 | G02 | ⬜ |
| P4-13 | 流式下载统一 `.part` + rename + 取消/清理 | M07/M08/M09 | ✅ 下载已有；取消/缓存清理待补 |
| P4-14 | 音频无播放器时的下载/分享兜底 | 边界 | ✅ 文案 + 下载/分享 |
| P4-15 | 不支持视频格式原因提示 | 边界 | 🟡 基础错误未展示 |

---

### P5 导入与收件箱

| ID | 任务 | 对应编号 | 状态 |
|---|---|---|---|
| P5-01 | 文件选择器多选上传、进度、取消 | I01 | ✅ |
| P5-02 | 系统分享目标 ACTION_SEND/SEND_MULTIPLE | I02 | ✅ |
| P5-03 | 原生接收持久化、私有目录、pending JSON | I03 | ✅ |
| P5-04 | 冷/热启动消费与 `share.available` | I04 | ✅ |
| P5-05 | 去重窗口、processed event 300 条、文件名清洗 | I05 | ✅ |
| P5-06 | 离线收件箱索引、文件/文本列表 | I06 | ✅ |
| P5-07 | 自动上传、逐项重试、部分成功原因 | I07 | ✅ |
| P5-08 | 删除本机收件、确认 | I08 | ✅ |
| P5-09 | 手机分享来源标记（响应 ID / SHA-256） | I09 | ✅ |
| P5-10 | `share_tags.json` 按稳定库作用域、2000 条上限 | I10 | ✅ |
| P5-11 | `peek → persist → ack` 协议与进程中断重放 | 迁移 | ✅ |
| P5-12 | 后台持续上传调度（如纳入范围需单列） | 新能力 | ⬜ 未承诺 |

**P5 出口条件**：单文件、多文件、文本、混合、冷/热启动、离线、杀进程恢复、部分失败全部有真机记录；不重复入箱、不丢待上传内容。

---

### P6 全量验证与发布候选

| ID | 任务 | 产出 | 状态 |
|---|---|---|---|
| P6-01 | 功能迁移验收表逐条回填（C/B/Q/M/G/I/R/U/E） | `reports/移动端功能迁移验收表.md` | ⬜ |
| P6-02 | 双真机（arm64 主力 + 低资源）验证 | 验收记录 | ⬜ |
| P6-03 | 覆盖升级、迁移失败可重试、数据不丢 | 升级报告 | ⬜ |
| P6-04 | 30 分钟混合操作无崩溃/ANR/泄漏 | 稳定性记录 | ⬜ |
| P6-05 | 性能对照：首屏、滚动 p50/p95、内存、100MB 传输峰值、包体 | 性能报告 | ⬜ |
| P6-06 | release keystore、versionCode、ABI、产物命名 | CI/签名说明 | ⬜ |
| P6-07 | CI 干净构建 AAR + APK | workflow | ⬜ |
| P6-08 | 旧 Dart 测试语义迁移（query/zoom/tags/theme/menu/fallback） | Go/NSGo 测试 | 🟡 query/fallback/theme 已迁 |
| P6-09 | 应急构建与回滚说明 | 发布说明 | ⬜ |

**发布候选门槛**：P6-01 无未解释缺口；P6-02～P6-05 达标；P6-06/07 可复现。

---

### P7 正式切换与清理

| ID | 任务 | 状态 |
|---|---|---|
| P7-01 | NSGo 设为唯一移动实现 | ⬜ |
| P7-02 | 删除 `mobile/lib`、`pubspec.*`、Flutter 嵌入与插件 | ⬜ |
| P7-03 | 删除/改写 `.github/workflows/android.yml` | ⬜ |
| P7-04 | `mobile-nsgo` → `mobile` 目录迁移（独立提交） | ⬜ |
| P7-05 | 更新 API/README/根文档中的 Flutter 引用 | ⬜ |
| P7-06 | 保留有价值旧测试与历史报告，更新路径 | ⬜ |
| P7-07 | 发布实施报告与最终能力/性能记录 | ⬜ |

---

## 3. 下一批可立即执行的任务（建议顺序）

### 批次 A：把纵向切片做稳（1～2 天）

1. **P1-12**：把 token 写入宿主 SecureStore；`session.json` 只保留地址、库身份、时间。SessionManager 增加 `LoadToken/SaveToken/DeleteToken` 异步适配，并在连接、重连、断开、撤销时同步。
2. **P2-07 真机**：安装 debug APK，验证深链冷/热启动、手动地址、配对码、返回键、安全区、深色、输入法。
3. **P4-01/P4-04 真机**：大图、GIF、MP4、Range seek、快速翻页、后台恢复；记录不支持格式。
4. **P3-18**：WebSocket 前台恢复检查、断线重连后补刷新；把 `items_updated` 的 900ms 丢弃改为“合并 + 延迟校准”。
5. **P6-08**：把 `refresh_gate`、`folder_drawer`、`grid_pinch` 的旧测试语义转成 Go 测试或 NSGo 页面测试。

### 批次 B：连接与资源库完整（2～3 天）

1. 扫码页：原生扫码 host handler + 权限拒绝重试 + 闪光灯/切镜头。
2. 资源库列表/切换确认；切换后 `gallery/search/detail` 全部失效并回到首页。
3. 断开确认与撤销提示；历史连接删除后当前会话状态一致。
4. NSD 列表去重、超时、Wi-Fi/VPN/Tailscale 候选地址排序。

### 批次 C：浏览完整（3～4 天）

1. 月份分组 + 粘性头 + 稳定扁平索引；2～5 列捏合与锚点恢复。
2. 侧栏集合、统计、目录树；`/stats` 404 时显示“不可用”，不显示 0。
3. 高级筛选 UI（大小/评分/URL/备注/颜色/文件夹/标签）。
4. 长按多选、批量加标签/目录、逐项失败结果。
5. 废纸篓页、恢复、清空；使用历史页。

### 批次 D：媒体与导入（4～5 天）

1. 流式下载任务统一（下载/分享/剪贴板共用），取消、进度、`.part`、缓存清理。
2. 文件 URI 剪贴板、MediaStore/SAF 导出、非图片文件路径。
3. 文件多选上传、进度、取消、逐项失败。
4. 收件箱 UI + 自动上传 + 重试 + 文本上传为 `.txt`。
5. `peek → persist → ack` 接入 Go；测试杀进程恢复、重复事件、部分失败。
6. 来源标记与按稳定库作用域隔离。

### 批次 E：迁移与发布（3～5 天）

1. 定位旧 Flutter secure storage / support / documents / filesDir / cache 路径；写迁移器与样本。
2. 迁移连接/历史/使用历史/inbox/pending/share_tags；失败可重试，旧文件保留。
3. 实际已发 APK 覆盖升级；签名、versionCode、ABI 策略。
4. 双真机、30 分钟稳定性、性能对照。
5. CI、release keystore、应急构建与验收表。

---

## 4. 验收记录模板

每个功能编号按以下格式回填，最终汇总到 `reports/移动端功能迁移验收表.md`：

| 编号 | 新实现路径 | 自动测试 | 真机步骤 | 结果 | 缺口/备注 |
|---|---|---|---|---|---|
| C03 | `internal/ui/connect.go`、`transport.Pair` | `TestStatusPairAndErrors` | 错误码/正确码/超时 | 待真机 | 401 与超时文案需确认 |

“结果”只能是：通过 / 有条件通过 / 未通过 / 未测。不能留空。

---

## 5. 风险与待决策

| 风险/决策 | 影响 | 当前处理 |
|---|---|---|
| NSGo 早期版本 API 变动 | 升级可能破坏页面/服务 | 固定 commit；升级单独提交并跑完整测试 |
| Android 明文 HTTP | 商店审核与安全 | 宿主已允许 cleartext；考虑 network security config 限制私网 |
| `minSdk=28` | 旧设备覆盖减少 | 发布前确认已安装设备分布；记录用户影响 |
| token 仍在本地 JSON | 安全不合规 | P1-12 发布前必做 |
| 旧签名/versionCode 未知 | 无法保证覆盖升级 | P0-08 读取实际 APK；准备同证书更高 versionCode |
| 同名库冲突 | share_tags/使用历史串库 | 优先稳定库 ID；否则 legacy scope，不自动合并 |
| 分享 pending 的删除窗口 | 可能丢待上传 | 宿主已改 ack；Go 侧接入后做杀进程测试 |
| GIF/视频解码能力 | 部分格式不能播放 | 保留下载/分享兜底并给原因 |
| 后台上传 | 系统限制与范围未定 | 首版不承诺；若要做单列任务与电量/网络策略 |
| 无 iOS 工程 | 不能声称跨平台已交付 | iOS 作为后续独立交付 |

---

## 6. 本轮已完成文件

```text
mobile-nsgo/
  go.mod / go.sum / nsgo.yaml / main.go / README.md
  scripts/build-aar.sh
  internal/model/model.go
  internal/query/query.go + query_test.go
  internal/transport/{client,api,ws,errors}.go + transport_test.go
  internal/storage/{store,local}.go + storage_test.go
  internal/platform/platform.go
  internal/app/{gallery,session}.go + app_test.go
  internal/ui/{app,connect,gallery,search,detail}.go + app_test.go
  android/...  Kotlin 宿主、Manifest、Gradle
reports/移动端NSGo实施任务计划.md
```

下一步执行 **批次 A 的 P1-12 与 P0-06/P0-07 真机验证**；在真机结果回来前，不把任何功能编号标为“通过”。
