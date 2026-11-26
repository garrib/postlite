# Postlite 测试用例文档

版本：v0.1.0 · 日期：2026-11-25 · 状态：定稿（实现基线）

## 1. 测试范围与覆盖目标

| 模块 | 目标 |
|---|---|
| Go `internal/proxy` | 信封→http.Request 的构建保真；错误映射；截断；重定向/超时/跳过校验三个开关 |
| Go `internal/history` | CRUD、7 天清理、10000 条上限、原子写、损坏恢复 |
| TS `curlParse` | 方言级语料表：Chrome(macOS/Windows)/PowerShell/手写文档风格的 curl 全覆盖 |
| TS `params` / `body` | URL↔Query 双向同步；信封构建与文档 §1 一致 |
| 集成（Go） | httptest 上游 + 完整 handler：发送→落历史→列表→删除闭环 |

不测：前端视觉细节、浏览器兼容性（macOS Chrome/Safari 手工验收）。

## 2. Mock 策略

- Go：**httptest.Server** 作为上游，覆盖 http/自签 https/重定向/大响应；不 mock 标准库 HTTP 客户端（用真栈）。
- TS：解析与构建均为**纯函数**，零 mock。
- 时间相关：`history` 注入 `now func()` 假时钟；proxy 超时用真实短超时（50ms 级）。

## 3. 关键用例

### 3.1 Go proxy（表驱动）

| # | 用例 | 断言 |
|---|---|---|
| P1 | GET 无 body | 上游收到 GET、无 body、无自动 CT 头 |
| P2 | json body 未显式 CT | 自动 `Content-Type: application/json`；显式给则不覆盖 |
| P3 | urlencoded fields | body=`k=v&z=w`（enabled 过滤 + urlencode），CT 自动 |
| P4 | form-data 文本+文件字段 | 上游 ParseMultipartForm 得到同值文件名/内容/CT；boundary 自动 |
| P5 | form-data 文件无内容 | 发送空文件体（占位） |
| P6 | raw + mime | CT= 用户 mime；mime 空 → text/plain |
| P7 | binary | 字节与 base64 解码一致，CT 默认 octet-stream |
| P8 | Host header | `req.Host` 生效（上游读 Host 是用户值） |
| P9 | 用户给 Content-Length | 被忽略，不产生 400 |
| P10 | 重复同名 header | 上游收到两条 |
| P11 | 非 http(s) scheme | 400 invalid_request |
| P12 | 上游 404 | 200 信封、status=404（透传） |
| P13 | 3xx 不跟随（默认） | status=302，Location 头可见 |
| P14 | followRedirects | 拿到最终 200 |
| P15 | insecureSkipVerify | 自签 https 成功；不开启则 502 |
| P16 | timeoutMs=50 vs 上游 sleep 500ms | 504 upstream_timeout |
| P17 | 上游 body 21MB | truncated=true、sizeBytes=真实大小、捕获=20MB |
| P18 | header 值含 CR/LF | 400（Go 侧显式校验拒绝，见实现） |
| P19 | 超尺寸 text/base64/headers/fields | 各自 400 |
| P20 | HEAD/DELETE 带 body=none | 上游正常收到 |
| P21 | json 空 text | 上游无 body、无 Content-Type（v0.2.4 空=无 body） |

### 3.2 Go history（表驱动 + 假时钟）

| # | 用例 | 断言 |
|---|---|---|
| H1 | append→list 降序 | 最新在前 |
| H2 | 7 天边界：now-7d-1ms / now-7d+1ms | 前者清、后者留 |
| H3 | 启动时清理 | 加载后立即生效 |
| H4 | 10000 上限 | 超限从最旧端丢 |
| H5 | 损坏文件恢复 | 原件改名归档、列表为空、进程不崩 |
| H6 | 删除不存在 id | 204、无副作用 |
| H7 | 清空 | 204、文件 items=[] |
| H8 | 写入原子性 | save 后文件内容完整合法 JSON |
| H9 | preview 截断 | 256KB 按 rune 边界、previewTruncated 标记 |
| H10 | >1MB 文件字段 | 历史中占位、内容丢弃 |

### 3.3 TS curlParse（方言语料表，每行 = 输入 curl + 期望信封）

覆盖组：

| 组 | 代表用例 |
|---|---|
| A. macOS Chrome 复制 | 单行 `curl 'url' -H ... --data-raw ...`；多旗标多引号 |
| B. 反斜杠续行 | 多行 bash 命令、行尾 `\` |
| C. Windows cmd | `curl "url" ^` 续行、`^` 尾 |
| D. PowerShell | 反引号续行、`curl.exe` 前缀 |
| E. 引号机制 | `$'...'` 内 `\n\t` 解码；`"..."` 内 `\"`；单引号原样含空格；`${VAR}` 保留字面量+warning |
| F. 旗标语义 | 默认 GET；`-d`→POST；`-X PUT` 覆盖；`-G`+`--data-urlencode`→query；`--json`；多 `-d` `&` 拼接；`-d@file` warning |
| G. `-F` 全家 | `name=v` / `name=@f.png` / `;type=` / `name=<f`（各出 warning 正确） |
| H. 认证与杂项 | `-u` Basic；`-b` Cookie；`-A` UA；`-e` Referer；`--compressed`；`-k/-L/-m` → options |
| I. URL 变体 | 无 scheme（补 http:// + warning）；带 query；`--url` |
| J. 未知旗标 | `-x`、`-o`、`-sS` → warning、不中断 |
| K. 组合短旗标 | `-sSL`；`-kL`；`-vX POST` 集群中带参旗标取余串为参 |

### 3.4 TS 其他

| # | 用例 | 断言 |
|---|---|---|
| T1 | URL→params 解析 | `?a=1&b=2` → 2 行；再回写 URL 不变形 |
| T2 | params→URL 回写 | disabled 行不进 URL；空 key 行丢弃 |
| T3 | urlencoded/ form-data 信封构建 | 与 api.md §1 字段逐一对齐 |
| T4 | curl 导出 | 表单→curl 字符串可再被自己的 parser 还原（round-trip） |

### 3.5 平台桥 platform.ts

| # | 用例 | 断言 |
|---|---|---|
| PB1 | filenameFor CT→扩展名映射 | `application/json`→response.json；带 charset 后缀同样命中；未知/空→response.bin（自动化） |
| PB2 | copyText 剪贴板失败回退 execCommand | DOM 场景，标记 manual（浏览器/WKWebView 手工验） |
| PB3 | platformDownload 原生桥 | window.webkit 存在时发 postlite download 消息，否则 `<a download>`；vitest 仅验纯函数分支（manual） |

## 4. 手工 E2E（原生壳，脚本化执行）

| # | 场景 | 通过标准 |
|---|---|---|
| M1 | 双击 Postlite.app | 独立窗口（非浏览器）、Dock 图标、菜单栏 ⌘Q/⌘C/⌘V 可用 |
| M2 | 下载响应体 | NSSavePanel 原生保存面板，文件字节与响应一致 |
| M3 | 正常退出（关窗/⌘Q） | postlite-server 子进程随之消失 |
| M4 | 强杀壳（SIGKILL） | 子进程 ≤3s 自行退出（父进程监控） |
| M5 | 子进程意外死亡 | 弹原生错误提示后应用退出（manual） |
