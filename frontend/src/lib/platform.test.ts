import { describe, expect, it } from 'vitest'
import { filenameFor, isNativeApp } from './platform'

describe('platform（PB1/PB3 纯函数部分）', () => {
  it('PB1: filenameFor CT → 扩展名', () => {
    expect(filenameFor('application/json')).toBe('response.json')
    expect(filenameFor('application/json; charset=utf-8')).toBe('response.json')
    expect(filenameFor('Text/HTML')).toBe('response.html')
    expect(filenameFor('image/png')).toBe('response.png')
    expect(filenameFor('')).toBe('response.bin')
    expect(filenameFor('application/x-weird')).toBe('response.bin')
  })

  it('PB3: node 环境非原生 app（bridge 不存在）', () => {
    expect(isNativeApp()).toBe(false)
  })
})
