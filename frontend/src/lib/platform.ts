// 平台桥：.app（WKWebView）与浏览器两种形态的差异收口（design.md §4.3）。

type WebkitBridge = { messageHandlers: { postlite: { postMessage(msg: unknown): void } } }

function bridge(): WebkitBridge | null {
  const w = window as unknown as { webkit?: WebkitBridge }
  return w.webkit?.messageHandlers?.postlite ? w.webkit : null
}

/** 是否运行在原生 .app 内。 */
export function isNativeApp(): boolean {
  return typeof window !== 'undefined' && bridge() !== null
}

/** 响应体保存：.app 走 NSSavePanel 原生保存；浏览器回退 <a download>。 */
export function platformDownload(filename: string, base64: string): void {
  const b = bridge()
  if (b) {
    b.messageHandlers.postlite.postMessage({ name: 'download', filename, base64 })
    return
  }
  const bin = atob(base64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  const a = document.createElement('a')
  a.href = URL.createObjectURL(new Blob([bytes]))
  a.download = filename
  a.click()
  URL.revokeObjectURL(a.href)
}

/** 复制文本：clipboard API 失败时 execCommand 兜底（WKWebView 兼容）。 */
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      const ok = document.execCommand('copy')
      ta.remove()
      return ok
    } catch {
      return false
    }
  }
}

/** 发送选项（原生设置面板 ↔ Web 双向同步，design.md §4.3）。 */
export interface SendOptions {
  followRedirects: boolean
  insecureSkipVerify: boolean
  timeoutMs: number
}

/** Web → 原生：选项变更时推给原生面板。 */
export function postOptions(o: SendOptions): void {
  bridge()?.messageHandlers.postlite.postMessage({ name: 'options', ...o })
}

/** 原生 → Web：注册原生面板回调（Swift 侧 evaluateJavaScript 调 __postliteSetOptions）。 */
export function onNativeSetOptions(cb: (o: Partial<SendOptions>) => void): void {
  ;(window as unknown as { __postliteSetOptions?: unknown }).__postliteSetOptions = cb
}

/** 原生 → Web：原生面板清空历史成功后通知 Web 刷新列表。 */
export function onNativeRefreshHistory(cb: () => void): void {
  ;(window as unknown as { __postliteRefreshHistory?: unknown }).__postliteRefreshHistory = cb
}

/** 由 Content-Type 推断下载文件名扩展。 */
export function filenameFor(contentType: string): string {
  const ct = (contentType || '').split(';')[0].trim().toLowerCase()
  const ext: Record<string, string> = {
    'application/json': '.json',
    'text/html': '.html',
    'text/plain': '.txt',
    'text/css': '.css',
    'text/javascript': '.js',
    'application/javascript': '.js',
    'application/xml': '.xml',
    'text/xml': '.xml',
    'image/png': '.png',
    'image/jpeg': '.jpg',
    'image/gif': '.gif',
    'image/svg+xml': '.svg',
    'application/pdf': '.pdf',
    'application/zip': '.zip',
  }
  return 'response' + (ext[ct] ?? '.bin')
}
