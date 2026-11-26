// 历史回填归一化：后端 JSON omitempty 会省略空字段（headers/body.text/fields/options
// 均可能缺失），直接透传会让 BodyEditor 读 undefined 抛错（v0.2.6 修「回填后 Body 空白」）。
import { type Body, type BodyType, type Header, type Options, type RequestEnvelope } from './types'

export function normalizeBody(raw: Partial<Body> | undefined): Body {
  // 旧历史 JSON 运行时可能仍含 none（UI 类型联合已移除），按 string 比较后归一
  const legacyType = raw?.type as string | undefined
  const t: BodyType = legacyType === 'none' || !legacyType ? 'json' : (legacyType as BodyType)
  return {
    type: t,
    text: raw?.text ?? '',
    mime: raw?.mime ?? '',
    fields: (raw?.fields ?? []).map((f) => ({ ...f })),
  }
}

export function normalizeOptions(raw: Partial<Options> | undefined): Options {
  return {
    timeoutMs: raw?.timeoutMs ?? 0,
    followRedirects: raw?.followRedirects ?? false,
    insecureSkipVerify: raw?.insecureSkipVerify ?? false,
    disableCookies: raw?.disableCookies ?? false,
  }
}

/** 历史记录里的 request（字段可能被 omitempty 省略）→ 完整可编辑信封。 */
export function requestFromHistory(raw: Partial<RequestEnvelope> | undefined): RequestEnvelope {
  return {
    method: raw?.method || 'GET',
    url: raw?.url || '',
    headers: (raw?.headers ?? []).map((h: Partial<Header>) => ({ ...h, enabled: h.enabled !== false })) as Header[],
    body: normalizeBody(raw?.body),
    options: normalizeOptions(raw?.options),
  }
}
