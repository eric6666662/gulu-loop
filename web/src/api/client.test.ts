import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, request } from './client'

function stubFetch(impl: (url: string, init?: RequestInit) => Promise<Response>) {
  vi.stubGlobal('fetch', vi.fn(impl))
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('api/client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('拆信封：返回 data', async () => {
    stubFetch(async () => jsonResponse({ code: 200, message: 'ok', data: [{ serviceProvider: 'deepseek' }] }))

    const data = await request<{ serviceProvider: string }[]>('/api/v1/service-providers')

    expect(data).toEqual([{ serviceProvider: 'deepseek' }])
  })

  it('code 非 200 抛 ApiError，message 用后端中文', async () => {
    stubFetch(async () => jsonResponse({ code: 409, message: '服务商已存在：deepseek' }, 409))

    await expect(request('/api/v1/service-providers')).rejects.toMatchObject({
      name: 'ApiError',
      status: 409,
      message: '服务商已存在：deepseek',
    })
  })

  it('响应不是 JSON 也不崩，抛 ApiError 带状态码', async () => {
    stubFetch(async () => new Response('<html>502 Bad Gateway</html>', { status: 502 }))

    const err = await request('/api/v1/service-providers').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(502)
  })

  it('网络异常归一化成 ApiError(status=0)', async () => {
    stubFetch(async () => {
      throw new TypeError('Failed to fetch')
    })

    const err = await request('/api/v1/service-providers').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(0)
    expect((err as ApiError).message).toContain('连不上本地服务')
  })
})
