# Postlite

本机轻量 HTTP 调试工具 —— Postman 的最小替代集。粘贴 curl 直接发送、表单化编辑请求、平铺最近 7 天历史。免登录，单文件，macOS 双击即用。

## 特性

- **粘贴 cURL 即发**：兼容 macOS Chrome「Copy as cURL」、bash/zsh 续行、Windows cmd `^`、PowerShell 反引号等方言；解析不了的旗标进警告，绝不整体失败
- **表单化编辑**：方法 / URL / Query 参数 / Headers / Body（`none` · `json` · `x-www-form-urlencoded` · `form-data` · `raw` · `binary`）
- **本地代理转发**：请求由本地 Go 服务转发，天然没有 CORS / 混合内容限制；上游 4xx/5xx 原样透传展示
- **历史**：平铺最近 7 天，自动清理，可检索、单删、清空、一键回填（不做接口分组）
- **cURL 导出**：当前表单反向生成可粘贴回终端的 curl 命令，与导入构成 round-trip
- **免登录**：服务只绑定 `127.0.0.1`，绝不上听局域网——这就是它的安全边界

## 快速开始

```bash
make build          # 产出单二进制 bin/postlite
make app            # 产出 bin/Postlite.app（macOS 双击即用）
```

运行后自动打开浏览器（默认 `http://127.0.0.1:7300`，端口被占则自动 +1）。

| 环境变量 | 作用 |
|---|---|
| `POSTLITE_PORT` | 起始端口（默认 7300） |
| `POSTLITE_NO_OPEN=1` | 不自动打开浏览器 |
| `POSTLITE_DATA_DIR` | 覆盖数据目录（默认 `~/Library/Application Support/postlite`） |

## 安全模型

无鉴权是产品需求；信任模型 = **只有本机进程可访问**，由 loopback-only 绑定保证。历史文件权限 `0600`、原子写；响应捕获上限 20MB；仅转发 http/https。

## 技术栈

- 后端：Go 标准库，**零第三方依赖**（`go.mod` 无 require）
- 前端：Vue 3 + Vite + TypeScript，产物 go:embed 内嵌进二进制（gzip ≈ 38KB）
- 存储：单 JSON 文件（temp + rename 原子写，损坏自动归档恢复）

## 开发

```bash
make test           # Go + 前端全量测试
make run            # 构建并本地起服务
cd frontend && npm run dev   # Vite dev（5174，/api 代理到 7300）
```

## 目录结构

```
docs/            设计 / 接口 / 数据 / 测试用例 四份基线文档（文档先行）
frontend/        Vue 3 前端（src/lib 纯逻辑层 + src/components）
internal/proxy   信封构建与转发、错误映射、截断
internal/history 7 天保留的 JSON 存储
internal/server  路由 + 静态 + SPA 兜底
internal/config  环境变量配置
```

## 硬边界（v0.1 不做）

接口分组 / 集合 / 环境变量、登录、云同步、前置后置脚本、断言、Mock Server、WebSocket。
