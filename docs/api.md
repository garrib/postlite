# Postlite 接口文档

版本：v0.1.0 · 日期：2026-11-25 · 状态：定稿（实现基线）

## 0. 通用约定

- 服务仅监听 `127.0.0.1`；Content-Type 统一 `application/json`。
- 时间戳一律 Unix 毫秒。
- 业务错误统一信封：

```json
{ "error": { "code": "invalid_request", "message": "人类可读信息" } }
```

| HTTP | code | 场景 |
|---|---|---|
| 400 | `invalid_request` | 信封缺字段 / URL 非 http(s) / 方法为空 / 超尺寸限制 |
| 502 | `upstream_unreachable` | DNS 失败、连接拒绝、TLS 握手失败等传输错误 |
| 504 | `upstream_timeout` | 上游超时（含 options.timeoutMs 与内部 300s 硬顶） |
| 500 | `internal_error` | 服务端自身异常（序列化/落盘等） |

> 上游返回的 4xx/5xx **不是**这里的错误：状态码原样透传（见 2.2）。

## 1. 请求信封（前端 → 代理）

`POST /api/proxy`，Body：

```jsonc
{
  "method": "POST",              // 必填。任意 HTTP 动词（GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS/自定义）
  "url": "https://a.com/x",     // 必填。必须 http:// 或 https://
  "headers": [                   // 可选；顺序保留；重复名允许
    { "name": "Authorization", "value": "Bearer x", "enabled": true }
  ],
  "body": {                      // 可选，缺省等价 type=none
    "type": "none",              // none | json | x-www-form-urlencoded | form-data | raw | binary
    "text": "原始文本体",         // json / raw / none 之外的文本体（json 直接存文本，不强制合法性）
    "mime": "application/xml",   // 仅 raw 用；raw 默认 text/plain
    "fields": [                  // 仅 form-data / x-www-form-urlencoded 用
      {
        "name": "file1",         // 字段名
        "type": "text",          // text | file（仅 form-data 有 file）
        "value": "v",            // type=text 的值
        "enabled": true,
        "fileName": "a.png",     // type=file：文件名
        "contentType": "image/png", // type=file：MIME；缺省 application/octet-stream
        "contentBase64": "..."   // type=file：文件内容；不填则发送空文件体
      }
    ]
  },
  "options": {                   // 可选，字段均可缺省
    "timeoutMs": 0,              // 0 = 不限（服务端硬顶 300000）
    "followRedirects": false,    // true = 跟随 3xx
    "insecureSkipVerify": false  // true = 跳过 TLS 校验
  },
  "record": true                 // 缺省 true；false = 本次发送不记历史
}
```

落历史规则：成功与传输级失败（502/504）都落（失败条目带 `error` 字段，见数据实体文档）；`400 invalid_request` 属请求构造错误，不落历史。

### 尺寸与数量限制（超限 → 400 invalid_request）

| 项 | 限制 |
|---|---|
| `body.text` | ≤ 2 MB |
| 单个 `contentBase64` 解码后 | ≤ 14 MB |
| `headers` | ≤ 128 条；单条 value ≤ 64 KB |
| `fields` | ≤ 64 条 |
| `timeoutMs` | ≤ 300000 |

`headers`/`fields` 中 `name` 为空的行在发送时**忽略**（用于容忍表单空行）。

### 代理构建规则（语义契约）

- Header 名 `Host`（大小写不敏感）→ 设置 `req.Host`，不进 header 列表。
- 用户给的 `Content-Length` 一律忽略，由 body 长度推导。
- `form-data`：服务端生成 boundary；`Content-Type` 若用户未显式给，自动设 `multipart/form-data; boundary=...`。
- `json`：用户未显式给 `Content-Type` 时自动设 `application/json`。
- `x-www-form-urlencoded`：未显式给时自动设 `application/x-www-form-urlencoded`；只取 `enabled=true` 的 fields 按 name=urlencode 拼接。
- `raw`：未显式给时按 `mime` 设，`mime` 空 → `text/plain`。
- `binary`：未显式给时 `application/octet-stream`。
- `none`：GET/HEAD/DELETE 无 body；其余方法也允许无 body。

## 2. 代理响应（代理 → 前端）

### 2.1 成功（HTTP 200）

```jsonc
{
  "status": 201,                  // 上游真实状态码（含 3xx/4xx/5xx）
  "statusText": "Created",
  "headers": [ { "name": "Content-Type", "value": "application/json" } ],  // 同名头多值 = 多条
  "bodyText": "...",              // 响应体可无损 UTF-8 解码时给出
  "bodyBase64": "...",            // 恒有；bodyText 与它内容一致（截断时对应截断后内容）
  "isText": true,                 // 是否为可展示文本（UTF-8 合法）
  "truncated": false,             // true = 超过 20MB 捕获上限被截断
  "sizeBytes": 1024,              // 上游 Content-Length 可用时为完整原始大小；否则为实际捕获字节数
  "timeMs": 123                  // 从发起上游请求到读完 body 的耗时
}
```

### 2.2 失败

传输失败走第 0 节错误码（502/504），Body 为错误信封。**上游 4xx/5xx 走 2.1 成功信封**。

## 3. 历史接口

### 3.1 `GET /api/history`

→ `200`：

```jsonc
{ "items": [ /* HistoryRecord，按 createdAt 降序 */ ] }
```

### 3.2 `DELETE /api/history/:id`

→ `204`；id 不存在也返回 204（幂等）。

### 3.3 `DELETE /api/history`

清空全部历史 → `204`。

## 4. `GET /api/health`

→ `200` `{ "ok": true, "version": "0.1.0" }`（版本号与编译时注入一致）。

## 5. 静态资源

`/` 与未匹配路径 → 内嵌前端 `index.html`（SPA 兜底）；前端未构建时返回内置提示页。`/api/*` 之外的 GET 走静态文件，命中内嵌产物才返回，否则 404。
