import { describe, expect, it } from 'vitest'
import { normalizeCommand, parseCurl, tokenize } from './curlParse'
import type { Header } from './types'

function headersOf(r: { headers: Header[] }): Record<string, string> {
  const out: Record<string, string> = {}
  for (const h of r.headers) out[h.name] = h.value
  return out
}

// ———————————————————— 分词器 ————————————————————

describe('tokenize', () => {
  it('单引号原样保留空格', () => {
    expect(tokenize(`curl 'a b' -H 'X: y z'`)).toEqual(['curl', 'a b', '-H', 'X: y z'])
  })
  it('双引号转义 \\" \\\\ \\$ \\`', () => {
    expect(tokenize('"a\\"b\\\\c\\$d\\`e"')).toEqual(['a"b\\c$d`e'])
  })
  it("$'...' ANSI-C 解码", () => {
    expect(tokenize(`$'a\\nb\\tc'`)).toEqual(['a\nb\tc'])
  })
  it('反斜杠转义任意字符', () => {
    expect(tokenize('a\\ b')).toEqual(['a b'])
  })
  it('无引号正常分词', () => {
    expect(tokenize('-X POST https://a.b/c')).toEqual(['-X', 'POST', 'https://a.b/c'])
  })
})

// ———————————————————— 归一化 ————————————————————

describe('normalizeCommand', () => {
  it('bash 反斜杠续行', () => {
    expect(normalizeCommand(`curl 'https://a.b' \\\n  -H 'X: y'`)).toBe(`curl 'https://a.b' -H 'X: y'`)
  })
  it('cmd ^ 续行', () => {
    expect(normalizeCommand('curl "https://a.b" ^\n  -H "X: y"')).toBe('curl "https://a.b" -H "X: y"')
  })
  it('PowerShell 反引号续行', () => {
    expect(normalizeCommand('curl "https://a.b" `\n  -H "X: y"')).toBe('curl "https://a.b" -H "X: y"')
  })
  it('终端提示符 $ 被剥除', () => {
    expect(normalizeCommand('$ curl https://a.b')).toBe('curl https://a.b')
  })
})

// ———————————————————— 方言语料表（A-K） ————————————————————

describe('parseCurl', () => {
  it('A: macOS Chrome 复制（单行多旗标）', () => {
    const r = parseCurl(
      `curl 'https://api.example.com/v1/users?page=2' -H 'accept: application/json' -H 'Authorization: Bearer tok' --compressed`,
    )
    expect(r.method).toBe('GET')
    expect(r.url).toBe('https://api.example.com/v1/users?page=2')
    expect(r.headers.find((h) => h.name === 'Authorization')?.value).toBe('Bearer tok')
    expect(r.body.type).toBe('json')
    expect(r.body.text).toBe('')
    expect(headersOf(r)['accept']).toBe('application/json')
    expect(headersOf(r)['Accept-Encoding']).toBe('gzip, deflate')
  })

  it('A2: macOS Chrome POST + JSON', () => {
    const r = parseCurl(
      `curl 'https://api.example.com/v1/users' -X POST -H 'Content-Type: application/json' --data-raw '{"name":"中 文"}'`,
    )
    expect(r.method).toBe('POST')
    expect(r.body.type).toBe('json')
    expect(r.body.text).toBe('{"name":"中 文"}')
  })

  it('B: 反斜杠续行多行命令', () => {
    const r = parseCurl(`curl https://a.b/c \\\n  -X PUT \\\n  -H 'A: 1' \\\n  -d 'x=1'`)
    expect(r.method).toBe('PUT')
    expect(r.url).toBe('https://a.b/c')
    expect(r.body.type).toBe('x-www-form-urlencoded')
    expect(r.body.fields[0].name).toBe('x')
    expect(r.body.fields[0].value).toBe('1')
  })

  it('C: Windows cmd 续行', () => {
    const r = parseCurl('curl "http://a.b/c" ^\n  -H "X: y"')
    expect(r.url).toBe('http://a.b/c')
    expect(headersOf(r)['X']).toBe('y')
  })

  it('D: PowerShell 反引号 + curl.exe 前缀', () => {
    const r = parseCurl('curl.exe "http://a.b/c" `\n  -H "X-Win: 1"')
    expect(r.url).toBe('http://a.b/c')
    expect(headersOf(r)['X-Win']).toBe('1')
  })

  it('E: $\'...\' 换行进 body，"..." 转义，${VAR} 字面量+警告', () => {
    const r = parseCurl(`curl http://a.b/c -d $'a=1\\nb=2' -H "X-T: \${TOKEN}"`)
    expect(r.method).toBe('POST')
    expect(r.warnings.some((w) => w.includes('${TOKEN}'))).toBe(true)
    expect(headersOf(r)['X-T']).toBe('${TOKEN}')
  })

  it('F1: 默认 GET；-d 出现默认 POST；-X 覆盖', () => {
    expect(parseCurl('curl http://a.b/c').method).toBe('GET')
    expect(parseCurl('curl http://a.b/c -d a=1').method).toBe('POST')
    expect(parseCurl('curl http://a.b/c -d a=1 -X PUT').method).toBe('PUT')
  })

  it('F2: -G + --data-urlencode → query + GET', () => {
    const r = parseCurl(`curl -G 'http://a.b/c' --data-urlencode 'q=中 文'`)
    expect(r.method).toBe('GET')
    expect(r.url).toBe('http://a.b/c?q=%E4%B8%AD%20%E6%96%87')
    expect(r.body.type).toBe('json')
    expect(r.body.text).toBe('')
  })

  it('F3: --json 等价 -d + Accept/CT json', () => {
    const r = parseCurl(`curl https://a.b/c --json '{"a":1}'`)
    expect(r.method).toBe('POST')
    expect(r.body.type).toBe('json')
    expect(headersOf(r)['Accept']).toBe('application/json')
    expect(headersOf(r)['Content-Type']).toBe('application/json')
  })

  it('F4: 多个 -d 用 & 拼接', () => {
    const r = parseCurl(`curl http://a.b/c -d a=1 -d b=2`)
    expect(r.body.type).toBe('x-www-form-urlencoded')
    expect(r.body.fields.map((f) => `${f.name}=${f.value}`).join('&')).toBe('a=1&b=2')
  })

  it('F5: -d @file 警告并保留字面量', () => {
    const r = parseCurl(`curl http://a.b/c -d @data.json`)
    expect(r.warnings.some((w) => w.includes('@data.json'))).toBe(true)
    expect(r.body.text).toBe('@data.json')
    expect(r.body.type).toBe('raw')
  })

  it('G: -F 全家（text / @file / ;type= / <file）', () => {
    const r = parseCurl(`curl http://a.b/c -F 'k=v' -F 'f=@a.png;type=image/png' -F 'g=<b.txt'`)
    expect(r.body.type).toBe('form-data')
    const fields = r.body.fields
    expect(fields[0]).toMatchObject({ name: 'k', type: 'text', value: 'v' })
    expect(fields[1]).toMatchObject({ name: 'f', type: 'file', fileName: 'a.png', contentType: 'image/png' })
    expect(fields[2]).toMatchObject({ name: 'g', type: 'file', fileName: 'b.txt' })
    expect(r.warnings.length).toBeGreaterThanOrEqual(2) // 两个文件占位警告
  })

  it('H: -u Basic 认证 / -b Cookie / -A UA / -e Referer', () => {
    const r = parseCurl(`curl http://a.b/c -u 'user:p@ss' -b 'k=v; k2=v2' -A 'MyUA/1.0' -e 'http://ref.er'`)
    expect(headersOf(r)['Authorization']).toBe(`Basic ${btoa('user:p@ss')}`)
    expect(headersOf(r)['Cookie']).toBe('k=v; k2=v2')
    expect(headersOf(r)['User-Agent']).toBe('MyUA/1.0')
    expect(headersOf(r)['Referer']).toBe('http://ref.er')
  })

  it('H2: -k/-L/-m → options', () => {
    const r = parseCurl(`curl -kL -m 3.5 http://a.b/c`)
    expect(r.options.insecureSkipVerify).toBe(true)
    expect(r.options.followRedirects).toBe(true)
    expect(r.options.timeoutMs).toBe(3500)
  })

  it('I1: 无 scheme 补 http:// + 警告', () => {
    const r = parseCurl(`curl localhost:8080/api/v1`)
    expect(r.url).toBe('http://localhost:8080/api/v1')
    expect(r.warnings.some((w) => w.includes('http://'))).toBe(true)
  })

  it('I2: --url 指定', () => {
    const r = parseCurl(`curl --url http://a.b/c`)
    expect(r.url).toBe('http://a.b/c')
  })

  it('J: 未知旗标进警告不中断', () => {
    const r = parseCurl(`curl http://a.b/c -x http://proxy:1 -o /dev/null --no-buffer`)
    expect(r.url).toBe('http://a.b/c')
    expect(r.warnings.join('\n')).toContain('-x')
    expect(r.warnings.join('\n')).toContain('--no-buffer')
  })

  it('K: 组合短旗标 -sSL / -vX POST', () => {
    const r1 = parseCurl(`curl -sSL http://a.b/c`)
    expect(r1.options.followRedirects).toBe(true)
    expect(r1.url).toBe('http://a.b/c')

    const r2 = parseCurl(`curl -vX POST http://a.b/c`)
    expect(r2.method).toBe('POST')
  })

  it('边界: && 后的部分不解析', () => {
    const r = parseCurl(`curl http://a.b/c && echo done`)
    expect(r.url).toBe('http://a.b/c')
    expect(r.warnings.every((w) => !w.includes('echo'))).toBe(true)
  })

  it('边界: 2>/dev/null 被忽略', () => {
    const r = parseCurl(`curl http://a.b/c 2>/dev/null`)
    expect(r.url).toBe('http://a.b/c')
  })

  it('边界: JSON body 里含 & 也不误拆（CT json → json 类型）', () => {
    const r = parseCurl(`curl http://a.b/c -H 'Content-Type: application/json' -d '{"a":"x&y"}'`)
    expect(r.body.type).toBe('json')
    expect(r.body.text).toBe('{"a":"x&y"}')
  })
})
