# Postlite 设计文档

版本：v0.2.0 · 日期：2026-11-26 · 修订：原生 Mac 壳 + 浅色 Postman 风 UI

## 1. 定位

本机单用户轻量 HTTP 调试工具 —— Postman 的最小替代集。面向开发者日常「随手发一个请求」的场景：

- 粘贴 curl 命令直接发送
- 表单方式编辑请求（方法 / URL / Query / Headers / Body）
- 平铺的最近历史（不做接口分组）
- 免登录、单文件、双击即用（macOS 优先）

## 2. 范围

### 2.1 做（In）

| 能力 | 说明 |
|---|---|
| 原生 Mac 窗口 | WKWebView 壳 + Go server 子进程双进程模型；独立窗口、Dock 图标、原生菜单、原生保存面板，无浏览器痕迹 |
| curl 导入 | 粘贴 bash / zsh / Windows cmd / PowerShell 复制出的 curl 命令，解析为表单 |
| 表单化请求编辑 | 方法、URL、Query 参数表、Header 表、Body（5 种类型） |
| 代理发送 | 由本地 Go 服务转发请求，绕开浏览器 CORS / 混合内容限制 |
| 响应查看 | 状态码、耗时、大小、响应头、响应体（Pretty / Raw / 复制） |
| 历史 | 平铺、按时间倒序、可检索、可回填；保留最近 7 天，自动清理 |
| curl 导出 | 把当前表单反向生成 curl 命令（复制） |

### 2.2 不做（Out）

- 接口分组 / 集合 / 环境变量 / 全局变量
- 登录、多用户、团队协作、云同步
- 请求前/后脚本、断言、自动化测试
- Mock Server、WebSocket / GraphQL 专用编辑器
- cookie 管理（作为普通 Header 处理）

## 3. 业务规则

### 3.1 curl 导入

- 尽力而为（best-effort）：识别到的旗标映射进表单；**无法映射的旗标进 `warnings`，不报错不丢弃整个导入**。
- 语义对齐 curl 本体：
  - 默认方法 GET；出现 `-d/-F/--data*/--json` 时默认 POST；`-X` 显式覆盖。
  - `-G` 把 data 挪到 query string，方法 GET。
  - 多个 `-d` 用 `&` 拼接；`-u` 生成 Basic Authorization；`-b` 生成 Cookie 头；`-A/-e/--compressed` 映射对应 Header。
  - `-k/--insecure` → 跳过证书校验；`-L/--location` → 跟随重定向；`-m/--max-time` → 超时。
  - `-F` 的 `@file` / `<file` 因浏览器无法预填本地文件内容，生成文件占位字段 + 警告，由用户手动选择文件。
  - `-d @file` 同样无法读取，保留 `@file` 字面量 + 警告。
- 引号规则：`'...'` 原样、`"..."` 处理 `\" \\ \$` 转义、`$'...'` 解码 `\n \t \r` 等 ANSI-C 转义；`${VAR}` 无法展开，保留字面量 + 警告。
- 行继续符：bash `\`、cmd `^`、PowerShell 反引号 三种都支持。

### 3.2 请求发送

- 上游的 4xx/5xx 是**成功路径**：状态原样透传给前端展示。
- 真正的失败（DNS、拒连、TLS、超时）映射为结构化错误（见接口文档错误码）。
- 不缓存、不重试；同一时刻允许并发多个请求。
- 默认不跟随重定向（与 curl 无 `-L` 行为一致）、不跳过证书校验、无超时（内部硬顶 300s）。

### 3.3 历史保留

- 保留窗口：**7×24 小时**（以「条目创建时刻」计）。
- 清理时机：启动时、每次追加前（惰性），二者都只删 `now-7d` 之前的条目。
- 上限 10000 条（超出丢最旧），防止极端使用下文件膨胀。
- 每次发送自动落一条（含完整请求信封 + 响应摘要 + 文本预览）。
- 响应体只存**文本预览 ≤ 256KB**；请求中的文件内容 base64 存储，单字段超 1MB 原始数据则丢弃内容只留占位（防历史文件膨胀）。

### 3.4 请求生命周期状态机

```
idle ──send()──▶ sending ──成功──▶ done
                     │
                     └──失败──▶ error
done/error ──再次send──▶ sending（响应区切回 loading）
```

历史条目无状态，只有 `createdAt`。

## 4. 技术边界

### 4.1 选型

| 层 | 选择 | 理由 |
|---|---|---|
| 后端 | Go（标准库，**零第三方依赖**） | 单二进制、交叉编译、转发请求就是 net/http 本职 |
| 前端 | Vue 3 + Vite + TypeScript | 与作者既有栈一致；产物 go:embed 内嵌进二进制 |
| 原生壳 | Swift（AppKit + WKWebView，单文件 swiftc 直编，不经 SPM） | 真 Mac app 体验；体积增量 < 0.5MB |
| 存储 | 单个 JSON 文件（原子写） | 历史量级小（7 天个人使用），不值得引 DB |
| UI | 手写轻量 CSS，不引组件库 | 「轻量」是产品定位，产物体积敏感 |

产物形态：**`Postlite.app`**（`Contents/MacOS/` 下两个可执行文件——Swift 壳 `Postlite` + Go 服务 `postlite-server`；Dock 图标 + 原生菜单栏）。另有 CLI 形态：`bin/postlite` 直接运行，起服务后自动打开系统浏览器。

### 4.2 运行时约束

- **只绑定 127.0.0.1**，绝不监听局域网/通配地址 —— 免登录的安全边界即此。
- 默认端口 7300，被占用则依次 +1 重试（上限 +50），以实际端口打开浏览器。
- 双进程模型：`Postlite`（Swift 壳）启动 `postlite-server`（Go）子进程（`POSTLITE_NO_OPEN=1`），从其 stderr 日志解析实际监听地址后加载窗口；壳正常退出时 SIGTERM 子进程；壳被强杀（SIGKILL）时由 Go 侧父进程监控兜底。
- 仅允许转发 `http://` `https://`，其余 scheme 拒绝。
- 数据目录：`os.UserConfigDir()/postlite/`（macOS 为 `~/Library/Application Support/postlite/`）。
- 环境变量：`POSTLITE_PORT`（指定起点端口）、`POSTLITE_NO_OPEN=1`（不自动开浏览器）、`POSTLITE_DATA_DIR`（覆盖数据目录，供测试）、`POSTLITE_WATCH_PARENT=1`（父进程退出后服务自杀，原生壳专用）。

### 4.3 UI 风格（浅色 · Postman 风）

- 整体浅色：白底主区、`#FAFBFC` 侧栏、`#E4E6EA` 分隔线、`#21252B` 主文本、`#6B7280` 次级文本。
- 品牌主色深松绿 `#112A25`（即 `color(srgb 0.0658 0.1631 0.1447)`）：Send 按钮、Tab 激活下划线、输入框 focus ring、checkbox、logo 的「lite」。
- 品牌字标：Postlite —— Post 700 加粗（主文本色）、lite 300 细体（主色）。
- 方法配色：GET `#00A67D` · POST `#E8930C` · PUT `#2D8CFF` · PATCH `#8E5CE6` · DELETE `#EF4444` · 其他 `#6B7280`（历史列表、方法下拉同步着色）。
- 方法选择器为自绘下拉组件：按钮态 = 方法色粗体文本 + caret；展开列表逐项方法色 + 当前项勾选；点击外部/Esc 关闭。
- 发送选项（跟随重定向 / 跳过证书校验 / 超时 ms）在 **macOS 系统菜单栏**：应用菜单内「设置…（⌘,）」打开原生设置面板，与 Web 状态双向同步（Web → 原生走 bridge 消息 `options`；原生 → Web 走 `window.__postliteSetOptions`）；浏览器（CLI）形态无原生菜单，回退为左上角「设置」下拉。
- Params / Headers / x-www-form-urlencoded 表：**空表时显示一行空行**（占位符引导），输入即建行，不做纯空白；form-data 空态显示引导文案。
- 响应 Body 的 Pretty 视图对合法 JSON 做语法高亮：key / 字符串 / 数字 / 布尔 / null 分色；实现为先 HTML 转义再包 span（防注入），非法 JSON 按纯文本展示。
- form-data 编辑行为栅格行：启用框 / 字段名 / 类型（文本|文件）/ 值区 / 删除；文件字段选中后显示「图标 + 文件名 + 大小」chip，可「换」重选、可 ✕ 清除内容；底部「＋ 文本字段 / ＋ 文件字段」。
- 布局仿 Postman：左侧历史侧栏（280px，检索/回填）→ 中栏上请求区（URL 栏在顶 + Params/Headers/Body Tab，Body 类型为分段式胶囊）→ 下响应区（状态行 + Body/Headers Tab）；Toast 用深色浮层做对比。
- 原生桥（仅 .app 内生效）：`window.webkit.messageHandlers.postlite`；消息 `{name:'download', filename, base64}` → NSSavePanel 原生保存；浏览器形态回退 `<a download>`；剪贴板在 navigator.clipboard 失败时 execCommand 兜底。
- 应用图标：macOS Big Sur 风格 squircle（透明画布 10% 边距、圆角 22.37%），Postman 橙渐变底 + 白色纸飞机；`scripts/make-icon.swift` 生成 iconset 后 `iconutil` 转 icns。

## 5. 安全边界

- 无鉴权是产品需求（免登录）；信任模型 = **只有本机进程可访问**，由 loopback-only 绑定保证。
- 转发是用户主动行为（工具本职），无黑名单；但拒绝非 http(s) scheme。
- 响应捕获上限 20MB，超出截断并标记 `truncated`，防单响应打爆内存。
- 落盘文件权限 0600；写入用 temp+rename 原子替换，避免半截文件。

## 6. 验收标准

1. 粘贴 Chrome（macOS）「Copy as cURL」命令：方法/URL/Header/Body 全部正确还原，可一键发送成功。
2. 五种 Body 类型（none / json / x-www-form-urlencoded / form-data / raw / binary 中除 none 外任选）均可编辑并发送，上游收到的字节与表单语义一致。
3. 历史自动清理：超过 7 天的条目在启动/写入后不再出现；未过期条目不受影响。
4. `make app` 产出 `Postlite.app`：双击出现独立窗口（非浏览器）、Dock 图标与菜单栏可用、⌘C/⌘V/⌘Q 生效；`make build` 仍产出 CLI 单文件 `bin/postlite`。
5. `go test ./...` 与前端 `npm test` 全绿；curl 解析器有方言级语料表测试。
6. `nc -z` 确认服务仅监听 127.0.0.1，局域网 IP 不可达。
7. 强杀壳进程（SIGKILL）后，`postlite-server` 子进程 3 秒内自行退出（父进程监控）；响应体下载在 .app 内走 NSSavePanel 原生保存面板。
