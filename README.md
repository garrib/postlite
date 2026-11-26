# Postlite

本机轻量 HTTP 调试工具 —— Postman 的最小替代集。原生 Mac 窗口（非浏览器）、浅色 Postman 风 UI、粘贴 curl 直接发送、表单化编辑请求、平铺最近 7 天历史。免登录，双击即用。

## 特性

- **原生 Mac 应用**：`make app` 产出 `Postlite.app`——独立窗口（WKWebView 壳）、Dock 图标、原生菜单（⌘Q/⌘C/⌘V）、响应体下载走系统保存面板；不是「打开浏览器的网页」
- **浅色 Postman 风**：白底 + Postman 橙 `#FF6C37`，方法配色（GET 绿 / POST 黄 / PUT 蓝 / PATCH 紫 / DELETE 红），左侧历史侧栏 + 上请求下响应
- **粘贴 cURL 即发**：兼容 macOS Chrome「Copy as cURL」、bash/zsh 续行、Windows cmd `^`、PowerShell 反引号；解析不了的旗标进警告，绝不整体失败
- **表单化编辑**：方法 / URL / Query 参数 / Headers / Body（`none` · `json` · `x-www-form-urlencoded` · `form-data` · `raw` · `binary`）
- **本地代理转发**：请求由本地 Go 服务转发，无 CORS / 混合内容限制；上游 4xx/5xx 原样透传
- **历史**：平铺最近 7 天自动清理，可检索、单删、清空、一键回填（不做接口分组）
- **cURL 导出**：当前表单反向生成可粘贴回终端的 curl 命令，与导入构成 round-trip
- **免登录**：服务只绑定 `127.0.0.1`，绝不上听局域网——这就是它的安全边界

## 快速开始

```bash
make app    # 产出 bin/Postlite.app，双击即用
make build  # 或 CLI 形态：单文件 bin/postlite-server（运行后自动开浏览器）
```

| 环境变量 | 作用 |
|---|---|
| `POSTLITE_PORT` | 起始端口（默认 7300，占用自动 +1） |
| `POSTLITE_NO_OPEN=1` | 不自动打开浏览器 |
| `POSTLITE_DATA_DIR` | 覆盖数据目录（默认 `~/Library/Application Support/postlite`） |
| `POSTLITE_WATCH_PARENT=1` | 父进程退出后服务自杀（原生壳专用） |

## 架构

```
Postlite.app
├── Contents/MacOS/Postlite         # Swift 壳（WKWebView 窗口 + 菜单 + 原生保存面板）
│     └─ 拉起子进程，从日志解析实际端口后加载窗口
├── Contents/MacOS/postlite-server  # Go 服务（代理转发 + 历史 + 内嵌前端）
│     └─ 仅绑 127.0.0.1；父进程监控防孤儿
└── Contents/Resources/AppIcon.icns
```

- 后端：Go 标准库，**零第三方依赖**（`go.mod` 无 require）
- 前端：Vue 3 + Vite + TypeScript，产物 go:embed 内嵌（gzip ≈ 38KB）
- 原生壳：Swift 单文件（AppKit + WKWebView），`swiftc` 直编不经 SPM
- 存储：单 JSON 文件（temp + rename 原子写，损坏自动归档恢复）

## 开发

```bash
make test                        # Go + 前端全量测试
make run                         # CLI 形态本地起服务
cd frontend && npm run dev       # Vite dev（5174，/api 代理到 7300）
```

## 目录结构

```
docs/             设计 / 接口 / 数据 / 测试用例 四份基线文档（文档先行）
ui/               Swift 原生壳（单文件）
scripts/          图标生成脚本（CoreGraphics → iconset → icns）
frontend/         Vue 3 前端（src/lib 纯逻辑层 + src/components）
internal/proxy    信封构建与转发、错误映射、截断
internal/history  7 天保留的 JSON 存储
internal/server   路由 + 静态 + SPA 兜底
internal/config   环境变量配置
```

## 硬边界（v0.2 不做）

接口分组 / 集合 / 环境变量、登录、云同步、前置后置脚本、断言、Mock Server、WebSocket。
