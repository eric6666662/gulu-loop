import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './client'
import * as api from './providers'

function stubFetch(body: unknown, status = 200) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
    ),
  )
}

describe('api/providers · 探测接口', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('test(name) 打 POST /service-providers/{name}/test 并返回结果', async () => {
    stubFetch({ code: 200, message: 'ok', data: { ok: true, latencyMs: 312, modelCount: 8 } })

    const res = await api.test('deepseek')

    expect(res).toEqual({ ok: true, latencyMs: 312, modelCount: 8 })
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(url).toBe('/api/v1/service-providers/deepseek/test')
    expect(init.method).toBe('POST')
  })

  it('test(name) 502 时抛 ApiError，message 带上游原因', async () => {
    stubFetch({ code: 502, message: 'DeepSeek 连不上：上游返回 401：api_key 无效' }, 502)

    const err = await api.test('deepseek').catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(502)
    expect((err as ApiError).message).toContain('api_key 无效')
  })

  it('fetchModels 只把 models 数组返回，不把整个信封交给调用方', async () => {
    stubFetch({ code: 200, message: 'ok', data: { models: ['a', 'b'] } })

    const ids = await api.fetchModels({ baseUrl: 'https://api.x/v1', apiKey: 'sk-1' })

    expect(ids).toEqual(['a', 'b'])
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(url).toBe('/api/v1/service-providers/models:fetch')
    expect(JSON.parse(init.body)).toMatchObject({ baseUrl: 'https://api.x/v1', apiKey: 'sk-1' })
  })

  it('fetchModels 上游失败抛 ApiError', async () => {
    stubFetch({ code: 502, message: '上游不支持拉取模型列表，请手动填写' }, 502)

    await expect(api.fetchModels({ baseUrl: 'https://api.x/v1', apiKey: 'sk' })).rejects.toBeInstanceOf(ApiError)
  })
})
