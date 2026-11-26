import { describe, expect, it } from 'vitest'
import { normalizeBody, normalizeOptions, requestFromHistory } from './normalize'

describe('normalize（T5 历史回填归一化）', () => {
  it('T5a: headers 缺失 → []', () => {
    const r = requestFromHistory({ method: 'GET', url: 'http://x', body: { type: 'json' } } as any)
    expect(r.headers).toEqual([])
  })

  it('T5b: body.text/fields/mime 缺失 → 空默认', () => {
    const b = normalizeBody({ type: 'json' } as any)
    expect(b).toEqual({ type: 'json', text: '', mime: '', fields: [] })
    const b2 = normalizeBody(undefined)
    expect(b2.fields).toEqual([])
    expect(b2.text).toBe('')
  })

  it('T5c: type 缺失或 none → json', () => {
    expect(normalizeBody({} as any).type).toBe('json')
    expect(normalizeBody({ type: 'none' } as any).type).toBe('json')
  })

  it('T5d: options 缺失 → 安全默认', () => {
    expect(normalizeOptions(undefined)).toEqual({
      timeoutMs: 0,
      followRedirects: false,
      insecureSkipVerify: false,
      disableCookies: false,
    })
    expect(normalizeOptions({ timeoutMs: 500 } as any).timeoutMs).toBe(500)
  })

  it('T5e: 完整信封原样保留（enabled=false 不丢）', () => {
    const raw = {
      method: 'POST',
      url: 'http://a.b/c',
      headers: [{ name: 'X-Off', value: 'v', enabled: false }],
      body: {
        type: 'form-data',
        text: '',
        mime: '',
        fields: [{ name: 'f', type: 'file', value: '', enabled: true, fileName: 'a.bin', contentType: '', contentBase64: 'AAAA' }],
      },
      options: { timeoutMs: 3000, followRedirects: true, insecureSkipVerify: false },
    }
    const r = requestFromHistory(raw as any)
    expect(r.method).toBe('POST')
    expect(r.headers[0].enabled).toBe(false)
    expect(r.body.fields[0]).toEqual(raw.body.fields[0])
    expect(r.options.timeoutMs).toBe(3000)
    expect(r.options.followRedirects).toBe(true)
  })
})
