// 表单 → curl 导出（design.md §2.1：与解析器构成 round-trip）。

import type { RequestEnvelope } from './types'

/** shell 单引号包裹；内部 ' 转成 '\''。 */
export function shQuote(s: string): string {
  return "'" + s.replace(/'/g, "'\\''") + "'"
}

/** 生成可直接粘贴回终端的 curl 命令。 */
export function toCurl(env: RequestEnvelope): string {
  const parts: string[] = ['curl']

  if (env.method !== 'GET') parts.push('-X', env.method.toUpperCase())

  parts.push(shQuote(env.url))

  for (const h of env.headers) {
    if (!h.enabled || h.name === '') continue
    parts.push('-H', shQuote(`${h.name}: ${h.value}`))
  }

  // body 语义需要显式 Content-Type 的类型：用户没给就补（保证 round-trip 及真实语义）。
  const hasCT = env.headers.some((h) => h.enabled && h.name.toLowerCase() === 'content-type')
  if (!hasCT && env.body.type === 'json') {
    parts.push('-H', shQuote('Content-Type: application/json'))
  }
  if (!hasCT && env.body.type === 'raw' && env.body.mime !== '') {
    parts.push('-H', shQuote(`Content-Type: ${env.body.mime}`))
  }

  switch (env.body.type) {
    case 'none':
      break
    case 'json':
    case 'raw':
    case 'x-www-form-urlencoded': {
      const text = env.body.type === 'x-www-form-urlencoded'
        ? env.body.fields
            .filter((f) => f.enabled && f.name !== '')
            .map((f) => `${encodeURIComponent(f.name)}=${encodeURIComponent(f.value)}`)
            .join('&')
        : env.body.text
      if (text !== '') parts.push('--data-raw', shQuote(text))
      break
    }
    case 'form-data':
      for (const f of env.body.fields) {
        if (!f.enabled || f.name === '') continue
        if (f.type === 'file') {
          const extra = f.contentType !== '' ? `;type=${f.contentType}` : ''
          parts.push('-F', shQuote(`${f.name}=@${f.fileName || f.name}${extra}`))
        } else {
          parts.push('-F', shQuote(`${f.name}=${f.value}`))
        }
      }
      break
    case 'binary':
      // 二进制无法内联进 curl 文本，导出占位说明
      parts.push('# 二进制 body 无法导出为文本（请在表单中重新选择文件）')
      break
  }

  if (env.options.followRedirects) parts.push('-L')
  if (env.options.insecureSkipVerify) parts.push('-k')
  if (env.options.timeoutMs > 0) parts.push('--max-time', String(Math.round(env.options.timeoutMs / 100) / 10))

  return parts.join(' ')
}
