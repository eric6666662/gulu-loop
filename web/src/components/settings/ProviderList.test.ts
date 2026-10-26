import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderList from './ProviderList.vue'
import type { Provider } from '../../api/providers'

function provider(name: string): Provider {
  return {
    serviceProvider: name,
    website: `https://${name}.com`,
    baseUrl: `https://api.${name}.com/v1`,
    model: 'm1',
    models: [{ id: 'm1', name: 'm1' }],
    hasApiKey: true,
    isActive: false,
    createdAt: '2026-10-26T00:00:00Z',
    updatedAt: '2026-10-26T00:00:00Z',
  }
}

describe('ProviderList', () => {
  it('每个服务商渲染一行，顺序与入参一致', () => {
    const w = mount(ProviderList, { props: { providers: [provider('deepseek'), provider('moonshot')] } })

    const rows = w.findAll('[data-provider-list] > li')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('deepseek')
    expect(rows[1].text()).toContain('moonshot')
  })

  it('行内 edit 事件带上对应的服务商', async () => {
    const w = mount(ProviderList, { props: { providers: [provider('deepseek'), provider('moonshot')] } })

    await w.findAll('[data-edit]')[1].trigger('click')

    expect(w.emitted('edit')?.[0]?.[0]).toMatchObject({ serviceProvider: 'moonshot' })
  })
})

describe('ProviderList · 激活（ADR-0015）', () => {
  it('行内 activate 事件带上对应的服务商', async () => {
    const p1 = provider('deepseek')
    const p2 = provider('moonshot')
    const w = mount(ProviderList, { props: { providers: [p1, p2] } })

    await w.findAll('[data-activate]')[1].trigger('click')

    expect(w.emitted('activate')?.[0]).toEqual([p2])
  })
})
