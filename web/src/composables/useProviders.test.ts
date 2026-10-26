import { afterEach, describe, expect, it, vi } from 'vitest'
import { useProviders } from './useProviders'
import * as api from '../api/providers'

function provider(over: Partial<api.Provider>): api.Provider {
  return {
    serviceProvider: 'deepseek',
    website: 'https://platform.deepseek.com',
    baseUrl: 'https://api.deepseek.com/v1',
    model: 'deepseek-v4-pro',
    models: [{ id: 'deepseek-v4-pro', name: 'V4 Pro' }],
    hasApiKey: true,
    createdAt: '2026-10-26T00:00:00Z',
    updatedAt: '2026-10-26T00:00:00Z',
    ...over,
  }
}

describe('useProviders', () => {
  afterEach(() => vi.restoreAllMocks())

  it('models 由 providers 派生：值取 id、标签取 name、带服务商标识', async () => {
    vi.spyOn(api, 'list').mockResolvedValue([
      provider({
        serviceProvider: 'deepseek',
        models: [
          { id: 'deepseek-v4-pro', name: 'V4 Pro' },
          { id: 'deepseek-v4-flash', name: 'V4 Flash' },
        ],
      }),
      provider({
        serviceProvider: 'moonshot',
        models: [{ id: 'kimi-k2', name: 'kimi-k2' }],
      }),
    ])

    const { models, load } = useProviders()
    await load()

    expect(models.value.map((m) => m.id)).toEqual(['deepseek-v4-pro', 'deepseek-v4-flash', 'kimi-k2'])
    expect(models.value[0]).toMatchObject({ id: 'deepseek-v4-pro', name: 'V4 Pro', serviceProvider: 'deepseek' })
    expect(models.value[2].serviceProvider).toBe('moonshot')
  })

  it('加载失败时 error 有值、providers 保持空', async () => {
    vi.spyOn(api, 'list').mockRejectedValue(new Error('boom'))

    const { providers, error, load } = useProviders()
    await load()

    expect(providers.value).toEqual([])
    expect(error.value).toBe('boom')
  })

  it('create 成功后 providers 与 models 都包含新服务商', async () => {
    vi.spyOn(api, 'list').mockResolvedValue([])
    vi.spyOn(api, 'create').mockResolvedValue(
      provider({ serviceProvider: 'openai', models: [{ id: 'gpt-4o', name: 'GPT-4o' }] }),
    )

    const { providers, models, load, create } = useProviders()
    await load()

    const created = await create({
      serviceProvider: 'openai',
      website: '',
      baseUrl: 'https://api.openai.com/v1',
      apiKey: 'sk',
      model: 'gpt-4o',
      models: [{ id: 'gpt-4o', name: 'GPT-4o' }],
    })

    expect(created.serviceProvider).toBe('openai')
    expect(providers.value.map((p) => p.serviceProvider)).toEqual(['openai'])
    expect(models.value.map((m) => m.id)).toEqual(['gpt-4o'])
  })

  it('update 成功后替换该条并重算 models', async () => {
    vi.spyOn(api, 'list').mockResolvedValue([
      provider({ serviceProvider: 'deepseek', models: [{ id: 'deepseek-v4-pro', name: 'V4 Pro' }] }),
    ])
    vi.spyOn(api, 'update').mockResolvedValue(
      provider({ serviceProvider: 'deepseek', models: [{ id: 'deepseek-v4-flash', name: 'V4 Flash' }] }),
    )

    const { providers, models, load, update } = useProviders()
    await load()
    expect(models.value.map((m) => m.id)).toEqual(['deepseek-v4-pro'])

    await update('deepseek', {
      serviceProvider: 'deepseek',
      website: '',
      baseUrl: 'https://api.deepseek.com/v1',
      apiKey: '',
      model: 'deepseek-v4-flash',
      models: [{ id: 'deepseek-v4-flash', name: 'V4 Flash' }],
    })

    expect(providers.value).toHaveLength(1)
    expect(models.value.map((m) => m.id)).toEqual(['deepseek-v4-flash'])
  })

  it('create 失败时抛错且不改本地列表（表单错误由页面展示）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue([])
    vi.spyOn(api, 'create').mockRejectedValue(new Error('服务商已存在：openai'))

    const { providers, load, create } = useProviders()
    await load()

    await expect(
      create({
        serviceProvider: 'openai',
        website: '',
        baseUrl: 'https://api.openai.com/v1',
        apiKey: 'sk',
        model: 'gpt-4o',
        models: [{ id: 'gpt-4o', name: 'GPT-4o' }],
      }),
    ).rejects.toThrow('服务商已存在：openai')
    expect(providers.value).toEqual([])
  })

  it('test(name) 返回连通结果且不自己弹提示（由页面决定）', async () => {
    vi.spyOn(api, 'test').mockResolvedValue({ ok: true, latencyMs: 312, modelCount: 8 })

    const { test } = useProviders()
    const res = await test('deepseek')

    expect(res).toEqual({ ok: true, latencyMs: 312, modelCount: 8 })
    expect(api.test).toHaveBeenCalledWith('deepseek')
  })

  it('remove 后该服务商的模型从 models 里消失', async () => {
    vi.spyOn(api, 'list').mockResolvedValue([
      provider({ serviceProvider: 'deepseek' }),
      provider({ serviceProvider: 'moonshot', models: [{ id: 'kimi-k2', name: 'kimi-k2' }] }),
    ])
    vi.spyOn(api, 'remove').mockResolvedValue(undefined)

    const { models, load, remove } = useProviders()
    await load()
    expect(models.value).toHaveLength(2)

    await remove('deepseek')

    expect(models.value.map((m) => m.serviceProvider)).toEqual(['moonshot'])
  })
})
