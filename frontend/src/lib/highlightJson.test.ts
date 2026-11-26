import { describe, expect, it } from 'vitest'
import { highlightJson } from './highlightJson'

describe('highlightJson', () => {
  it('key / 字符串 / 数字 / 布尔 / null 分色', () => {
    const out = highlightJson('{"a":1,"b":[true,null]}')
    expect(out).toContain('<span class="j-key">"a":</span>')
    expect(out).toContain('<span class="j-key">"b":</span>')
    expect(out).toContain('<span class="j-num">1</span>')
    expect(out).toContain('<span class="j-bool">true</span>')
    expect(out).toContain('<span class="j-null">null</span>')
  })

  it('字符串值标 j-str，含 < > 时先转义（防注入）', () => {
    const out = highlightJson('{"s":"x<y&z"}')
    expect(out).toContain('<span class="j-str">"x&lt;y&amp;z"</span>')
    expect(out).not.toContain('<y')
  })

  it('script 标签整体被转义，不产生可执行 HTML', () => {
    const out = highlightJson('{"a":"</script><img src=x onerror=alert(1)>"}')
    expect(out).not.toContain('</script>')
    expect(out).toContain('&lt;/script&gt;')
  })

  it('负数与指数', () => {
    const out = highlightJson('[-1.5e3, 42]')
    expect(out).toContain('<span class="j-num">-1.5e3</span>')
    expect(out).toContain('<span class="j-num">42</span>')
  })

  it('纯数字输入', () => {
    expect(highlightJson('123')).toBe('<span class="j-num">123</span>')
  })
})
