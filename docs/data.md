# Postlite 数据实体文档

版本：v0.3.0 · 日期：2026-11-26 · 状态：定稿（实现基线）

## 1. 存储位置与形态

- 历史文件：`<dataDir>/history.json`；Cookie 持久化：`<dataDir>/cookies.json`。
  - macOS：`~/Library/Application Support/postlite/`
  - 测试/特殊场景：`POSTLITE_DATA_DIR` 覆盖。
- 文件权限 `0600`；写入 = 临时文件 + `rename` 原子替换（同目录）。
- 无 DB、无 schema 迁移：文件顶层带 `version` 字段为将来留余量。

```jsonc
{
  "version": 2,
  "items": [ /* HistoryRecord，createdAt 升序存储（写出时保持有序） */ ]
}
```

cookies.json（v0.3.0）：

```jsonc
{
  "version": 1,
  "cookies": [ /* http.Cookie 子集：name/value/domain/path/expires/secure/httpOnly */ ]
}
```

## 2. HistoryRecord 字段

```jsonc
{
  "id": "m0g8xk3c-1a2b",      // 创建时刻毫秒（36 进制）+ "-" + 4 位随机 hex
  "createdAt": 1732531200000, // Unix 毫秒
  "pinned": false,            // v0.3.0：钉选豁免清理与数量淘汰，列表置顶（omitempty）
  "request": { /* 请求信封（同接口文档 §1，但恒无 record 字段） */ },
  "response": {               // 发送成功时存在
    "status": 200,
    "statusText": "OK",
    "sizeBytes": 1024,        // 上游完整大小（可能被截断前的大小）
    "timeMs": 123,
    "contentType": "application/json",  // 上游 Content-Type 原文，缺省空串
    "previewText": "...",     // 文本响应预览，≤ 256KB；二进制响应无此字段
    "previewTruncated": false // 预览是否因 256KB 被裁
  },
  "error": "连接失败…"        // 发送失败时存在（与 response 互斥）
}
```

### 约束

| 字段 | 约束 |
|---|---|
| `id` | 唯一；时间前缀保证升序可比较 |
| `createdAt` | 写入时由服务端生成，客户端不可指定 |
| `request` | 完整保留（回填运行的唯一来源） |
| `response.previewText` | 仅 `isText=true` 的响应存；≤ 256KB（UTF-8 截断按 rune 边界） |
| `request` 内 `contentBase64` | 单字段解码后 > 1MB 时**丢弃内容、保留占位**（回填时 UI 提示重新选择文件） |

## 3. 保留策略

- 窗口：`now - 7×24h`（以 `createdAt` 计）。
- 时机：进程启动加载后、每次 Append 前。惰性清理。
- **钉选豁免**（v0.3.0）：`pinned=true` 的条目不参与清理；数量上限 10000 条仍强制，淘汰顺序为最旧未钉选优先，全部钉选才动最旧钉选。
- 删除语义：
  - `DELETE /api/history/:id` 幂等；
  - `DELETE /api/history` 全清；
  - 两者与追加都走「内存更新 → 原子落盘」同一路径。

## 4. 容错

| 异常 | 行为 |
|---|---|
| 文件不存在 | 视为空历史，正常启动 |
| JSON 损坏 | 把原文件改名为 `history.json.corrupt-<ms>` 归档，从空开始（绝不 crash、绝不删原件） |
| 写入失败（磁盘满等） | 内存状态保持、不回滚（下次写入重试）。`/api/history*` 相关操作返回 `500 internal_error`；`/api/proxy` 落历史失败**不阻塞响应**，仅 stderr 告警 |
| `version` 未知 | 按能读多少读多少（向前兼容策略：忽略未知字段） |
