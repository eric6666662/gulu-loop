/**
 * 后端 HTTP 客户端：唯一认识 `{code, message, data}` 信封的地方（02-前端设计.md §2.1）。
 *
 * 上层拿到的永远是 `data`；非 200 一律抛 ApiError，message 直接用后端中文（可直出给用户）。
 */

/** 后端统一信封 */
interface Envelope<T> {
  code: number
  message: string
  data?: T
}

export class ApiError extends Error {
  /** HTTP 状态码；0 表示请求根本没发出去（本地服务没起、网络断） */
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

/** 同源请求：SPA 由 Go 服务提供，不需要 baseURL 配置 */
export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      ...init,
      headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    })
  } catch {
    throw new ApiError(0, '连不上本地服务，检查 127.0.0.1:1112 是否还在跑')
  }

  let body: unknown
  try {
    body = await res.json()
  } catch {
    // 上游返回了非 JSON（代理错误页 / 网关 502）：不崩，按状态码报错
    throw new ApiError(res.status, `请求失败：HTTP ${res.status}`)
  }

  const env = body as Envelope<T>
  if (!res.ok || env.code !== 200) {
    throw new ApiError(res.status, env.message || `请求失败：HTTP ${res.status}`)
  }
  return env.data as T
}
