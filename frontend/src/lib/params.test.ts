import { describe, expect, it } from 'vitest'
import { buildUrl, parseUrl } from './params'

describe('parseUrl / buildUrl（T1/T2）', () => {
  it('T1: URL → 行 → URL 不变形（含中文）', () => {
    const url = 'http://a.b/x?a=1&b=%E4%B8%AD'
    const { base, rows, hash } = parseUrl(url)
    expect(base).toBe('http://a.b/x')
    expect(rows).toEqual([
      { key: 'a', value: '1', enabled: true },
      { key: 'b', value: '中', enabled: true },
    ])
    expect(buildUrl(base, rows, hash)).toBe(url)
  })

  it('T1: 空 query / 无 query', () => {
    expect(parseUrl('http://a.b/x').rows).toEqual([])
    expect(parseUrl('http://a.b/x?').rows).toEqual([])
    expect(buildUrl('http://a.b/x', [], '')).toBe('http://a.b/x')
  })

  it('T1: hash 保留', () => {
    const { base, rows, hash } = parseUrl('http://a.b/x?a=1#sec')
    expect(hash).toBe('#sec')
    expect(buildUrl(base, rows, hash)).toBe('http://a.b/x?a=1#sec')
  })

  it('T1: 值里含 = 不误切', () => {
    const { rows } = parseUrl('http://a.b/x?b=c=d')
    expect(rows[0]).toEqual({ key: 'b', value: 'c=d', enabled: true })
  })

  it('T2: disabled 行与空 key 行不进 URL', () => {
    const out = buildUrl(
      'http://a.b/x',
      [
        { key: 'a', value: '1', enabled: true },
        { key: 'b', value: '2', enabled: false },
        { key: '', value: '3', enabled: true },
        { key: 'd', value: '', enabled: true },
      ],
      '',
    )
    expect(out).toBe('http://a.b/x?a=1&d=')
  })

  it('往返: 特殊字符编码稳定', () => {
    const url = 'http://a.b/x?q=%E4%B8%AD%26%3D%20'
    const { base, rows, hash } = parseUrl(url)
    expect(rows[0].value).toBe('中&= ')
    expect(buildUrl(base, rows, hash)).toBe(url)
  })
})
