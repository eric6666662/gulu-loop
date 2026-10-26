import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderDetail from './ProviderDetail.vue'
import type { Provider } from '../../api/providers'

const provider: Provider = {
  serviceProvider: 'deepseek',
  website: 'https://platform.deepseek.com',
  baseUrl: 'https://api.deepseek.com/v1',
  model: 'deepseek-v4-pro',
  models: [
    { id: 'deepseek-v4-flash', name: 'V4 Flash' },
    { id: 'deepseek-v4-pro', name: 'V4 Pro' },
  ],
  hasApiKey: true,
  createdAt: '2026-10-26T00:00:00Z',
  updatedAt: '2026-10-26T00:00:00Z',
}

describe('ProviderDetail', () => {
  it('编辑态：名称只读、api_key 提示「已配置，留空则不修改」', () => {
    const w = mount(ProviderDetail, { props: { provider } })

    expect((w.find('[data-name]').element as HTMLInputElement).readOnly).toBe(true)
    expect(w.find('[data-api-key]').attributes('placeholder')).toBe('已配置，留空则不修改')
    expect(w.text()).toContain('2 个 · 默认 deepseek-v4-pro')
  })

  it('新增态：名称可编辑、api_key 必填提示、模型清单为空', () => {
    const w = mount(ProviderDetail, { props: { provider: null } })

    expect((w.find('[data-name]').element as HTMLInputElement).readOnly).toBe(false)
    expect(w.findAll('[data-model-id]')).toHaveLength(0)
  })

  it('Base URL 为空时拦下提交并给出可直出的文案', async () => {
    const w = mount(ProviderDetail, { props: { provider: { ...provider, baseUrl: '' } } })

    await w.find('[data-save]').trigger('click')

    expect(w.emitted('save')).toBeUndefined()
    expect(w.text()).toContain('Base URL 不能为空')
    expect(w.find('[data-base-url]').attributes('aria-invalid')).toBe('true')
  })

  it('模型清单为空时拦下提交', async () => {
    const w = mount(ProviderDetail, { props: { provider: { ...provider, models: [], model: '' } } })

    await w.find('[data-save]').trigger('click')

    expect(w.emitted('save')).toBeUndefined()
    expect(w.text()).toContain('至少配置一个模型')
  })

  it('合法时 emit save，apiKey 留空原样传空串（后端 = 不修改）', async () => {
    const w = mount(ProviderDetail, { props: { provider } })

    await w.find('[data-save]').trigger('click')

    const payload = w.emitted('save')?.[0]?.[0] as Record<string, unknown>
    expect(payload).toMatchObject({
      serviceProvider: 'deepseek',
      baseUrl: 'https://api.deepseek.com/v1',
      apiKey: '',
      model: 'deepseek-v4-pro',
    })
    expect((payload.models as unknown[]).length).toBe(2)
  })

  it('新增态：加一行模型并填标识后，save 带上该模型', async () => {
    const w = mount(ProviderDetail, { props: { provider: null } })

    await w.find('[data-name]').setValue('zzz')
    await w.find('[data-base-url]').setValue('https://api.zzz.com/v1')
    await w.find('[data-api-key]').setValue('sk-3')
    await w.find('[data-model-add]').trigger('click')
    await w.find('[data-model-id]').setValue('zzz-1')
    await w.find('[data-save]').trigger('click')

    const payload = w.emitted('save')?.[0]?.[0] as { models: { id: string }[]; model: string }
    expect(payload?.models?.[0]?.id).toBe('zzz-1')
    expect(payload?.model).toBe('zzz-1')
  })

  it('点「拉取模型列表」emit fetch-models，带上当前表单的连接信息', async () => {
    const w = mount(ProviderDetail, { props: { provider } })

    await w.find('[data-fetch-models]').trigger('click')

    expect(w.emitted('fetch-models')?.[0]?.[0]).toMatchObject({
      serviceProvider: 'deepseek',
      baseUrl: 'https://api.deepseek.com/v1',
      apiKey: '',
    })
  })

  it('addModels 追加勾选的模型并去重（展示名缺省 = 标识）', async () => {
    const w = mount(ProviderDetail, { props: { provider } })

    w.vm.addModels(['deepseek-chat', 'deepseek-v4-pro'])
    await w.vm.$nextTick()

    const ids = w.findAll('[data-model-id]').map((i) => (i.element as HTMLInputElement).value)
    expect(ids).toEqual(['deepseek-v4-flash', 'deepseek-v4-pro', 'deepseek-chat'])

    await w.find('[data-save]').trigger('click')
    const payload = w.emitted('save')?.[0]?.[0] as { models: { id: string; name: string }[] }
    expect(payload.models.find((m) => m.id === 'deepseek-chat')?.name).toBe('deepseek-chat')
  })

  it('后端错误（重名）显示在表单里', () => {
    const w = mount(ProviderDetail, { props: { provider, error: '服务商已存在：deepseek' } })

    expect(w.text()).toContain('服务商已存在：deepseek')
  })

  it('点「← 服务商列表」与「取消」都 emit back', async () => {
    const w = mount(ProviderDetail, { props: { provider } })

    await w.find('[data-back]').trigger('click')
    await w.find('[data-cancel]').trigger('click')

    expect(w.emitted('back')).toHaveLength(2)
  })
})
