import { describe, expect, it } from 'vitest'
import { toCurl } from './curlExport'
import { parseCurl } from './curlParse'
import { emptyRequest } from './types'

describe('toCurl（T4 round-trip）', () => {
  it('GET 默认省略 -X', () => {
    const env = emptyRequest()
    env.url = 'https://a.b/c?x=1'
    expect(toCurl(env)).toBe(`curl 'https://a.b/c?x=1'`)
  })

  it('POST + headers + json body round-trip', () => {
    const env = emptyRequest()
    env.method = 'POST'
    env.url = 'https://a.b/c'
    env.headers.push({ name: 'Authorization', value: 'Bearer t', enabled: true })
    env.body = { type: 'json', text: '{"a":1}', mime: '', fields: [] }
    const cmd = toCurl(env)
    expect(cmd).toContain(`-X POST`)
    expect(cmd).toContain(`--data-raw '{"a":1}'`)

    const back = parseCurl(cmd)
    expect(back.method).toBe('POST')
    expect(back.url).toBe('https://a.b/c')
    expect(back.body.type).toBe('json')
    expect(back.body.text).toBe('{"a":1}')
    expect(back.headers.find((h) => h.name === 'Authorization')?.value).toBe('Bearer t')
  })

  it('options 导出：-k -L --max-time', () => {
    const env = emptyRequest()
    env.url = 'http://a.b/c'
    env.options = { timeoutMs: 3500, followRedirects: true, insecureSkipVerify: true }
    const cmd = toCurl(env)
    expect(cmd).toContain('-L')
    expect(cmd).toContain('-k')
    expect(cmd).toContain('--max-time 3.5')
  })

  it('单引号转义：URL 含引号不炸', () => {
    const env = emptyRequest()
    env.url = "http://a.b/c?x='"
    expect(toCurl(env)).toContain(`'http://a.b/c?x='\\'''`)
  })

  it('urlencoded 字段导出并 round-trip', () => {
    const env = emptyRequest()
    env.method = 'POST'
    env.url = 'http://a.b/c'
    env.body = {
      type: 'x-www-form-urlencoded',
      text: '',
      mime: '',
      fields: [
        { name: 'a', type: 'text', value: '1', enabled: true, fileName: '', contentType: '', contentBase64: '' },
        { name: 'b', type: 'text', value: '中 文', enabled: true, fileName: '', contentType: '', contentBase64: '' },
      ],
    }
    const back = parseCurl(toCurl(env))
    expect(back.body.type).toBe('x-www-form-urlencoded')
    expect(back.body.fields.map((f) => `${f.name}=${f.value}`).join('&')).toBe('a=1&b=中 文')
  })
})
