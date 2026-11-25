// curl 命令解析器：bash/zsh、Windows cmd、PowerShell 三方言尽力解析（docs/design.md §3.1）。
// 纯函数、零依赖；解析不了的旗标进 warnings，绝不整体失败。

import { emptyRequest, type BodyField, type RequestEnvelope } from './types'

export interface ParsedCurl extends RequestEnvelope {
  warnings: string[]
}

// ———————————————————— 第一步：归一化 ————————————————————

// 三种行继续符（bash `\`、cmd `^`、PowerShell 反引号）+ 终端提示符。
// 续行点周围的空白一并吞掉；引号内数据不受影响（分词在 tokenize 中按引号处理）。
export function normalizeCommand(input: string): string {
  let s = input.replace(/\r\n?/g, '\n').trim()
  s = s.replace(/^\$\s+/gm, '') // 行首终端提示符 $（粘贴自终端）
  s = s.replace(/[ \t]*\\\n[ \t]*/g, ' ') // bash/zsh 续行
  s = s.replace(/[ \t]*\^\n[ \t]*/g, ' ') // cmd 续行
  s = s.replace(/[ \t]*`\n[ \t]*/g, ' ') // PowerShell 续行
  s = s.replace(/[ \t]*\n[ \t]*/g, ' ') // 其余裸换行（文本折行）按空格
  return s.trim()
}

// ———————————————————— 第二步：类 shell 分词 ————————————————————

// 规则：'...' 原样；"..." 处理 \" \\ \$ \` 转义；$'...' 解 ANSI-C 转义；\x 转义下一字符。
export function tokenize(s: string): string[] {
  const out: string[] = []
  let cur = ''
  let i = 0
  const flush = () => {
    if (cur !== '') {
      out.push(cur)
      cur = ''
    }
  }

  while (i < s.length) {
    const c = s[i]

    // $'...' ANSI-C 引号
    if (c === '$' && s[i + 1] === "'") {
      i += 2
      while (i < s.length && s[i] !== "'") {
        if (s[i] === '\\' && i + 1 < s.length) {
          const [decoded, adv] = decodeAnsiC(s, i + 1)
          cur += decoded
          i += 1 + adv
        } else {
          cur += s[i]
          i++
        }
      }
      i++ // 跳过收尾 '
      continue
    }

    if (c === "'") {
      // 单引号：完全原样
      i++
      while (i < s.length && s[i] !== "'") {
        cur += s[i]
        i++
      }
      i++
      continue
    }

    if (c === '"') {
      i++
      while (i < s.length && s[i] !== '"') {
        if (s[i] === '\\' && i + 1 < s.length && '"\\$`'.includes(s[i + 1])) {
          cur += s[i + 1]
          i += 2
        } else {
          cur += s[i]
          i++
        }
      }
      i++
      continue
    }

    if (c === '\\' && i + 1 < s.length) {
      // 反斜杠转义下一字符（含续行已被归一化吃掉的换行场景）
      cur += s[i + 1]
      i += 2
      continue
    }

    if (c === ' ' || c === '\t') {
      flush()
      i++
      continue
    }

    cur += c
    i++
  }
  flush()
  return out
}

// decodeAnsiC 解码 $'...' 内以 s[start] 开头的转义序列，返回 [文本, 消耗字符数]。
function decodeAnsiC(s: string, start: number): [string, number] {
  const c = s[start]
  switch (c) {
    case 'n':
      return ['\n', 1]
    case 't':
      return ['\t', 1]
    case 'r':
      return ['\r', 1]
    case '0':
      return ['\0', 1]
    case 'a':
      return ['\x07', 1]
    case 'b':
      return ['\b', 1]
    case 'f':
      return ['\f', 1]
    case 'v':
      return ['\v', 1]
    case '\\':
      return ['\\', 1]
    case "'":
      return ["'", 1]
    case '"':
      return ['"', 1]
    default:
      return ['\\' + c, 1] // 未知转义保持字面
  }
}

// ———————————————————— 第三步：旗标映射 ————————————————————

interface ParseCtx {
  env: RequestEnvelope
  warnings: string[]
  dataParts: string[] // -d 累积
  urlencodeParts: string[] // --data-urlencode 累积
  formParts: BodyField[] // -F 累积
  jsonUsed: boolean
  getFlag: boolean // -G
  methodSet: boolean
}

function warn(ctx: ParseCtx, msg: string) {
  ctx.warnings.push(msg)
}

function addHeader(ctx: ParseCtx, name: string, value: string) {
  // 去掉同名旧头（后写覆盖，贴近 curl 行为）
  const lower = name.toLowerCase()
  ctx.env.headers = ctx.env.headers.filter((h) => h.name.toLowerCase() !== lower)
  ctx.env.headers.push({ name, value, enabled: true })
}

// 参数性短旗标表（集群展开时需要知道哪些字母带参）。
// 注：带参旗标在集群循环里按「余串非空即参，否则吃下一 token」统一处理。

function applyHeaderArg(ctx: ParseCtx, raw: string) {
  const colon = raw.indexOf(':')
  if (colon <= 0) {
    warn(ctx, `忽略无法解析的 header 参数：${raw}`)
    return
  }
  const name = raw.slice(0, colon).trim()
  const value = raw.slice(colon + 1).trim()
  if (name === '' || name.startsWith('-')) {
    warn(ctx, `忽略无法解析的 header 参数：${raw}`)
    return
  }
  addHeader(ctx, name, value)
}

function applyDataArg(ctx: ParseCtx, raw: string, flagName: string) {
  if (raw.startsWith('@')) {
    warn(ctx, `-${flagName} @文件 无法在网页里读取本地文件，保留字面量 ${raw}，请手动改写`)
    ctx.dataParts.push(raw)
    return
  }
  ctx.dataParts.push(raw)
}

function applyFormArg(ctx: ParseCtx, raw: string) {
  const eq = raw.indexOf('=')
  if (eq <= 0) {
    warn(ctx, `忽略无法解析的 -F 参数：${raw}`)
    return
  }
  const name = raw.slice(0, eq)
  let rest = raw.slice(eq + 1)

  if (rest.startsWith('@') || rest.startsWith('<')) {
    const at = rest[0]
    rest = rest.slice(1)
    // -F 'f=@a.png;type=image/png' / -F 'f=@a.png;filename=b.png'
    let fileName = rest
    let contentType = ''
    if (rest.includes(';')) {
      const segs = rest.split(';')
      fileName = segs[0]
      for (const seg of segs.slice(1)) {
        const [k, v] = seg.split('=')
        if (k.trim() === 'type') contentType = v.trim()
        if (k.trim() === 'filename') fileName = v.trim()
      }
    }
    const f: BodyField = {
      name,
      type: 'file',
      value: '',
      enabled: true,
      fileName,
      contentType,
      contentBase64: '',
    }
    ctx.formParts.push(f)
    warn(ctx, at === '<' ? `-F <文件 无法预填内容：字段 ${name}（文件 ${fileName}）请手动选择` : `-F @文件 无法预填内容：字段 ${name}（文件 ${fileName}）请手动选择`)
  } else {
    const f: BodyField = { name, type: 'text', value: rest, enabled: true, fileName: '', contentType: '', contentBase64: '' }
    ctx.formParts.push(f)
  }
}

function applyUserArg(ctx: ParseCtx, raw: string) {
  const cred = raw.includes(':') ? raw : raw + ':'
  addHeader(ctx, 'Authorization', `Basic ${utf8ToB64(cred)}`)
}

function utf8ToB64(s: string): string {
  return btoa(String.fromCharCode(...new TextEncoder().encode(s)))
}

function stopTokens(t: string): boolean {
  return t === '&&' || t === ';' || t === '||' || t === '|'
}

function looksLikeRedirect(t: string): boolean {
  return /^\d*(>{1,2})/.test(t)
}

/** parseCurl 主入口。 */
export function parseCurl(input: string): ParsedCurl {
  const normalized = normalizeCommand(input)
  const tokens = tokenize(normalized)

  const ctx: ParseCtx = {
    env: emptyRequest(),
    warnings: [],
    dataParts: [],
    urlencodeParts: [],
    formParts: [],
    jsonUsed: false,
    getFlag: false,
    methodSet: false,
  }

  // 定位 curl / curl.exe 起点
  let start = tokens.findIndex((t) => /^curl(\.exe)?$/i.test(t))
  if (start === -1) {
    warn(ctx, '未识别到 curl 前缀，按参数尽力解析')
    start = 0
  } else {
    start++
  }

  const LONG: Record<string, (v: string) => void> = {
    'request': (v) => setMethod(ctx, v),
    'header': (v) => applyHeaderArg(ctx, v),
    'data': (v) => applyDataArg(ctx, v, 'd'),
    'data-raw': (v) => applyDataArg(ctx, v, 'd'),
    'data-binary': (v) => applyDataArg(ctx, v, 'd'),
    'data-ascii': (v) => applyDataArg(ctx, v, 'd'),
    'data-urlencode': (v) => ctx.urlencodeParts.push(v),
    'form': (v) => applyFormArg(ctx, v),
    'user': (v) => applyUserArg(ctx, v),
    'user-agent': (v) => addHeader(ctx, 'User-Agent', v),
    'referer': (v) => addHeader(ctx, 'Referer', v),
    'cookie': (v) => addHeader(ctx, 'Cookie', v),
    'max-time': (v) => {
      const sec = parseFloat(v)
      if (Number.isFinite(sec)) ctx.env.options.timeoutMs = Math.round(sec * 1000)
    },
    'url': (v) => setUrl(ctx, v),
    'json': (v) => {
      ctx.jsonUsed = true
      ctx.dataParts.push(v)
      addHeader(ctx, 'Accept', 'application/json')
    },
    'upload-file': () => warn(ctx, '忽略 --upload-file（不支持文件上传的 curl 语义）'),
    'output': () => warn(ctx, '忽略 -o/--output（网页端自动展示响应）'),
    'proxy': () => warn(ctx, '忽略 -x/--proxy（不支持代理转发）'),
  }

  const SHORT_BOOL: Record<string, () => void> = {
    'k': () => (ctx.env.options.insecureSkipVerify = true),
    'L': () => (ctx.env.options.followRedirects = true),
    'G': () => (ctx.getFlag = true),
    's': () => {},
    'S': () => {},
    'i': () => {},
    'v': () => {},
    'compressed': () => addHeader(ctx, 'Accept-Encoding', 'gzip, deflate'),
  }

  let i = start
  for (; i < tokens.length; i++) {
    let t = tokens[i]

    if (stopTokens(t)) break // shell 连接符后不再解析
    if (looksLikeRedirect(t)) {
      warn(ctx, `忽略 shell 重定向符号：${t}`)
      continue
    }

    if (!t.startsWith('-') || t === '-') {
      setUrl(ctx, t)
      continue
    }

    if (t.startsWith('--')) {
      // --flag=value | --flag value
      let name = t.slice(2)
      let value: string | undefined
      const eq = name.indexOf('=')
      if (eq >= 0) {
        value = name.slice(eq + 1)
        name = name.slice(0, eq)
      }
      if (name in LONG) {
        if (value === undefined) {
          // 按表内实现是否需要参数决定是否吃下一个 token
          value = takesArg(name) ? tokens[++i] : ''
        }
        LONG[name](value ?? '')
      } else if (BOOL_LONG.has(name)) {
        SHORT_BOOL[name]?.()
      } else {
        warn(ctx, `忽略未知旗标 --${name}`)
      }
      continue
    }

    // 短旗标（含集群 -sSL）
    const cluster = t.slice(1)
    for (let ci = 0; ci < cluster.length; ci++) {
      const ch = cluster[ci]
      if (ch === 'k' || ch === 'L' || ch === 'G' || ch === 's' || ch === 'S' || ch === 'i' || ch === 'v') {
        SHORT_BOOL[ch]?.()
        continue
      }
      // 带参短旗标：余串非空即参，否则吃下一 token
      let arg = cluster.slice(ci + 1)
      if (arg === '') arg = tokens[++i] ?? ''
      if (arg === '') {
        warn(ctx, `旗标 -${ch} 缺少参数`)
        break
      }
      switch (ch) {
        case 'X':
          setMethod(ctx, arg)
          break
        case 'H':
          applyHeaderArg(ctx, arg)
          break
        case 'd':
          applyDataArg(ctx, arg, 'd')
          break
        case 'F':
          applyFormArg(ctx, arg)
          break
        case 'u':
          applyUserArg(ctx, arg)
          break
        case 'A':
          addHeader(ctx, 'User-Agent', arg)
          break
        case 'e':
          addHeader(ctx, 'Referer', arg)
          break
        case 'b':
          addHeader(ctx, 'Cookie', arg)
          break
        case 'm': {
          const sec = parseFloat(arg)
          if (Number.isFinite(sec)) ctx.env.options.timeoutMs = Math.round(sec * 1000)
          break
        }
        default:
          warn(ctx, `忽略未知旗标 -${ch}`)
      }
      break // 带参旗标吞掉余串后集群结束
    }
  }

  // 默认方法规则（design.md §3.1）：有 -d/-F/--data*/--json 且无 -X 时默认 POST；-G 优先 GET。
  const hadData =
    ctx.dataParts.length + ctx.urlencodeParts.length > 0 || ctx.formParts.length > 0 || ctx.jsonUsed

  // -G：data 挪到 query string（须在 finalizeBody 前，否则 body 会被重复填一份）
  if (ctx.getFlag) {
    const all = [...ctx.dataParts, ...ctx.urlencodeParts]
    if (all.length > 0) {
      if (ctx.env.url === '') {
        warn(ctx, '-G 但未识别到 URL，data 参数被丢弃')
      } else {
        const qs = all
          .map((kv) => {
            const eq = kv.indexOf('=')
            if (eq < 0) return `${encodeURIComponent(kv)}=`
            return `${encodeURIComponent(kv.slice(0, eq))}=${encodeURIComponent(kv.slice(eq + 1))}`
          })
          .join('&')
        ctx.env.url += (ctx.env.url.includes('?') ? '&' : '?') + qs
      }
    }
    ctx.dataParts = []
    ctx.urlencodeParts = []
    if (!ctx.methodSet) setMethod(ctx, 'GET') // -X 显式给出时优先 -X
  } else if (!ctx.methodSet && hadData) {
    setMethod(ctx, 'POST')
  }

  finalizeBody(ctx)
  checkEnvVars(ctx, tokens)

  const result: ParsedCurl = { ...ctx.env, warnings: ctx.warnings }
  return result
}

const BOOL_LONG = new Set(['k', 'insecure', 'L', 'location', 'location-trusted', 'G', 'get', 's', 'silent', 'S', 'show-error', 'i', 'include', 'v', 'verbose', 'compressed'])

function takesArg(name: string): boolean {
  return !BOOL_LONG.has(name)
}

function setMethod(ctx: ParseCtx, m: string) {
  ctx.env.method = m.toUpperCase()
  ctx.methodSet = true
}

function setUrl(ctx: ParseCtx, u: string) {
  if (ctx.env.url !== '') {
    // 已经有 URL，再出现裸 token 大概率是 shell 噪声
    warn(ctx, `忽略多余的位置参数：${u}`)
    return
  }
  if (/^https?:\/\//i.test(u)) {
    ctx.env.url = u
    return
  }
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(u)) {
    ctx.env.url = u
    warn(ctx, `协议 ${u.slice(0, u.indexOf(':'))}: 非 http/https，发送时将被拒绝`)
    return
  }
  // 无协议（含 localhost:8080/x 形态）：按 curl 惯例补 http://
  ctx.env.url = 'http://' + u
  warn(ctx, `URL 缺协议，已按 curl 惯例补 http://：${u}`)
}

function contentTypeOf(ctx: ParseCtx): string {
  const h = ctx.env.headers.find((x) => x.name.toLowerCase() === 'content-type')
  return h ? h.value.toLowerCase() : ''
}

// finalizeBody 决定 body 类型的最终形态（design.md §3.1）。
function finalizeBody(ctx: ParseCtx) {
  const ct = contentTypeOf(ctx)

  if (ctx.formParts.length > 0) {
    ctx.env.body.type = 'form-data'
    ctx.env.body.fields = ctx.formParts
    return
  }

  const dataParts = [...ctx.dataParts, ...ctx.urlencodeParts]
  if (dataParts.length === 0) {
    // curl --json 不带数据时也倾向 json 类型
    ctx.env.body.type = ctx.jsonUsed ? 'json' : 'none'
    return
  }

  const joined = dataParts.filter((d) => !d.startsWith('@')).join('&')
  const hasFileRef = dataParts.some((d) => d.startsWith('@'))
  if (hasFileRef && joined === '') {
    // 只有 @file 引用：保留字面量，按 raw 发送
    ctx.env.body.type = 'raw'
    ctx.env.body.text = dataParts.join('&')
    ensureCT(ctx, 'application/x-www-form-urlencoded')
    return
  }

  if (ctx.jsonUsed || ct.includes('json')) {
    ctx.env.body.type = 'json'
    ctx.env.body.text = joined
    ensureCT(ctx, 'application/json')
    return
  }

  // 尝试拆成 k=v 表（友好 UI）；失败则按 raw 发送（字节保真）。
  if (!hasFileRef && isSimpleKv(joined)) {
    ctx.env.body.type = 'x-www-form-urlencoded'
    ctx.env.body.fields = parseKvRows(joined)
    ensureCT(ctx, 'application/x-www-form-urlencoded')
    return
  }

  ctx.env.body.type = 'raw'
  ctx.env.body.text = joined
  ensureCT(ctx, 'application/x-www-form-urlencoded')
}

function isSimpleKv(s: string): boolean {
  if (!s.includes('=')) return false
  return s.split('&').every((part) => {
    const eq = part.indexOf('=')
    return eq >= 1 // 每段都有名字
  })
}

function parseKvRows(joined: string): BodyField[] {
  return joined.split('&').map((part) => {
    const eq = part.indexOf('=')
    const name = part.slice(0, eq)
    const raw = part.slice(eq + 1)
    // 尝试解码（代理端会重新编码），失败保留原文
    let value = raw
    try {
      value = decodeURIComponent(raw)
    } catch {
      /* 保留原文 */
    }
    return { name, value, type: 'text', enabled: true, fileName: '', contentType: '', contentBase64: '' } as BodyField
  })
}

function ensureCT(ctx: ParseCtx, ct: string) {
  if (!contentTypeOf(ctx)) addHeader(ctx, 'Content-Type', ct)
}

function checkEnvVars(ctx: ParseCtx, tokens: string[]) {
  for (const t of tokens) {
    if (/\$\{[A-Za-z_][A-Za-z0-9_]*\}/.test(t)) {
      warn(ctx, `检测到 ${t.match(/\$\{[A-Za-z_][A-Za-z0-9_]*\}/g)?.join(' ')} 无法展开，保留字面量`)
      return
    }
  }
}
