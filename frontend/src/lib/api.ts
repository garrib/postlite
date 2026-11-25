// 后端 API 客户端（接口契约见 docs/api.md）。

import type { ApiResponse, HistoryRecord, RequestEnvelope } from './types'

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public httpStatus: number,
  ) {
    super(message)
  }
}

async function parse(resp: Response): Promise<never | any> {
  if (!resp.ok) {
    let code = 'internal_error'
    let message = `HTTP ${resp.status}`
    try {
      const body = await resp.json()
      if (body?.error) {
        code = body.error.code ?? code
        message = body.error.message ?? message
      }
    } catch {
      /* 保留默认 */
    }
    throw new ApiError(code, message, resp.status)
  }
  return resp.json()
}

export async function sendRequest(env: RequestEnvelope): Promise<ApiResponse> {
  const resp = await fetch('/api/proxy', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(env),
  })
  return parse(resp) as Promise<ApiResponse>
}

export async function listHistory(): Promise<HistoryRecord[]> {
  const resp = await fetch('/api/history')
  const body = await parse(resp)
  return body.items as HistoryRecord[]
}

export async function deleteHistory(id: string): Promise<void> {
  const resp = await fetch(`/api/history/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (resp.status !== 204) throw new ApiError('internal_error', `删除失败 HTTP ${resp.status}`, resp.status)
}

export async function clearHistory(): Promise<void> {
  const resp = await fetch('/api/history', { method: 'DELETE' })
  if (resp.status !== 204) throw new ApiError('internal_error', `清空失败 HTTP ${resp.status}`, resp.status)
}

// fileToBase64 浏览器文件 → base64（form-data 文件字段 / binary body 用）。
export function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const rd = new FileReader()
    rd.onload = () => {
      const s = String(rd.result)
      resolve(s.slice(s.indexOf(',') + 1))
    }
    rd.onerror = () => reject(rd.error)
    rd.readAsDataURL(file)
  })
}
