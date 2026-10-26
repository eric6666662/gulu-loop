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
    isActive: false,
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

describe('useProviders · 激活的服务商（ADR-0015）', () => {
  afterEach(() => vi.restoreAllMocks())

  it('activeProvider = isActive 那条；没有则 null', async () => {
    vi.spyOn(api, 'list').mockResolvedValue([
      provider({ serviceProvider: 'deepseek', isActive: false }),
      provider({ serviceProvider: 'relay', isActive: true }),
    ])
    const p = useProviders()
    await p.load()

    expect(p.activeProvider.value?.serviceProvider).toBe('relay')

    vi.spyOn(api, 'list').mockResolvedValue([provider({ serviceProvider: 'deepseek', isActive: false })])
    await p.load()
    expect(p.activeProvider.value).toBeNull()
  })

  it('activate 成功后把返回项整条替换、其余 isActive 置 false（不重拉列表）', async () => {
    const listSpy = vi.spyOn(api, 'list').mockResolvedValue([
      provider({ serviceProvider: 'deepseek', isActive: true }),
      provider({ serviceProvider: 'relay', isActive: false }),
    ])
    const p = useProviders()
    await p.load()

    vi.spyOn(api, 'activate').mockResolvedValue(provider({ serviceProvider: 'relay', isActive: true }))

    const got = await p.activate('relay')

    expect(got.isActive).toBe(true)
    expect(p.activeProvider.value?.serviceProvider).toBe('relay')
    expect(p.providers.value.find((x) => x.serviceProvider === 'deepseek')?.isActive).toBe(false)
    expect(listSpy).toHaveBeenCalledTimes(1) // 只有最初那次 load
  })

  it('activate 失败时抛错且本地列表不变', async () => {
    vi.spyOn(api, 'list').mockResolvedValue([
      provider({ serviceProvider: 'deepseek', isActive: true }),
      provider({ serviceProvider: 'relay', isActive: false }),
    ])
    const p = useProviders()
    await p.load()
    vi.spyOn(api, 'activate').mockRejectedValue(new Error('服务商不存在'))

    await expect(p.activate('nope')).rejects.toThrow('服务商不存在')
    expect(p.activeProvider.value?.serviceProvider).toBe('deepseek')
  })

  it('删掉激活的那个后 activeProvider 变 null（落到无激活态）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue([
      provider({ serviceProvider: 'deepseek', isActive: true }),
      provider({ serviceProvider: 'relay', isActive: false }),
    ])
    const p = useProviders()
    await p.load()
    vi.spyOn(api, 'remove').mockResolvedValue(undefined)

    await p.remove('deepseek')

    expect(p.activeProvider.value).toBeNull()
  })
})
