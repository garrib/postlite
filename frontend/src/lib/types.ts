// 与 docs/api.md §1 请求信封逐字段对齐的前端类型。

/** PopSelect 通用下拉的选项（cls = 附加着色类，如 msel-get）。 */
export interface PopOption {
  value: string
  label: string
  cls?: string
}

export interface Header {
  name: string
  value: string
  enabled: boolean
}

export type BodyType = 'json' | 'x-www-form-urlencoded' | 'form-data' | 'raw' | 'binary'

export interface BodyField {
  name: string
  type: 'text' | 'file' // file 仅 form-data
  value: string
  enabled: boolean
  fileName: string
  contentType: string
  contentBase64: string
}

export interface Body {
  type: BodyType
  text: string
  mime: string
  fields: BodyField[]
}

export interface Options {
  timeoutMs: number // 0 = 不限
  followRedirects: boolean
  insecureSkipVerify: boolean
}

export interface RequestEnvelope {
  method: string
  url: string
  headers: Header[]
  body: Body
  options: Options
}

// ———————————————————— 响应信封（api.md §2） ————————————————————

export interface ResponseHeader {
  name: string
  value: string
}

export interface ApiResponse {
  status: number
  statusText: string
  headers: ResponseHeader[]
  bodyText: string
  bodyBase64: string
  isText: boolean
  truncated: boolean
  sizeBytes: number
  timeMs: number
}

// ———————————————————— 历史（data.md §2） ————————————————————

export interface HistoryResponseSummary {
  status: number
  statusText: string
  sizeBytes: number
  timeMs: number
  contentType: string
  previewText?: string
  previewTruncated?: boolean
}

export interface HistoryRecord {
  id: string
  createdAt: number
  request: RequestEnvelope
  response?: HistoryResponseSummary
  error?: string
}

// ———————————————————— 工厂 ————————————————————

export function emptyHeader(name = '', value = ''): Header {
  return { name, value, enabled: true }
}

export function emptyField(): BodyField {
  return { name: '', value: '', enabled: true, type: 'text', fileName: '', contentType: '', contentBase64: '' }
}

export function emptyBody(): Body {
  // v0.2.4 起无 none 类型：空文本即不发送 body（api.md 代理构建规则）
  return { type: 'json', text: '', mime: '', fields: [] }
}

export function emptyRequest(): RequestEnvelope {
  return { method: 'GET', url: '', headers: [], body: emptyBody(), options: { timeoutMs: 0, followRedirects: false, insecureSkipVerify: false } }
}
