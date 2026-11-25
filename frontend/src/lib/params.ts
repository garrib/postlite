// URL ↔ Query 参数表 双向同步（docs/test-cases.md §3.4 T1/T2）。

import type { BodyField, Header } from './types'

export interface ParamRow {
  key: string
  value: string
  enabled: boolean
}

/** 把 URL 拆成 base（无 query/hash）+ 参数行 + hash。 */
export function parseUrl(url: string): { base: string; rows: ParamRow[]; hash: string } {
  const hashIdx = url.indexOf('#')
  let hash = ''
  let rest = url
  if (hashIdx >= 0) {
    hash = url.slice(hashIdx)
    rest = url.slice(0, hashIdx)
  }

  const qIdx = rest.indexOf('?')
  let base = rest
  let query = ''
  if (qIdx >= 0) {
    base = rest.slice(0, qIdx)
    query = rest.slice(qIdx + 1)
  }

  const rows: ParamRow[] = []
  if (query !== '') {
    for (const part of query.split('&')) {
      if (part === '') continue
      const eq = part.indexOf('=')
      const rawKey = eq < 0 ? part : part.slice(0, eq)
      const rawVal = eq < 0 ? '' : part.slice(eq + 1)
      rows.push({ key: safeDecode(rawKey), value: safeDecode(rawVal), enabled: true })
    }
  }
  return { base, rows, hash }
}

/** 由 base + 参数行 + hash 重建 URL（disabled 行剔除，空 key 行丢弃）。 */
export function buildUrl(base: string, rows: ParamRow[], hash: string): string {
  const pairs = rows
    .filter((r) => r.enabled && r.key !== '')
    .map((r) => `${encodeURIComponent(r.key)}=${encodeURIComponent(r.value)}`)
    .join('&')
  let out = base
  if (pairs !== '') out += '?' + pairs
  if (hash) out += hash
  return out
}

function safeDecode(s: string): string {
  try {
    return decodeURIComponent(s)
  } catch {
    return s
  }
}

// ———————————————————— 表单行通用工具 ————————————————————

/** key/value 表（headers / params / urlencoded 共用）：按 name 分组取值。 */
export function rowKv(rows: Array<Header | BodyField>): Array<[string, string]> {
  return rows.filter((r) => r.enabled && r.name !== '').map((r) => [r.name, r.value] as [string, string])
}
