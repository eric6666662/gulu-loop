import { describe, expect, it } from 'vitest'
import { computed } from 'vue'
import { mount } from '@vue/test-utils'
import ProviderList from './ProviderList.vue'
import type { Provider } from '../../api/providers'

/** ProviderList 从 App provide 下来读激活项（判无激活横幅）；两种调用形态都收 */
function mountList(arg: Provider[] | { providers: Provider[] }) {
  const list = Array.isArray(arg) ? arg : arg.providers
  return mount(ProviderList, {
    props: { providers: list },
    global: {
      provide: {
        providers: {
          activeProvider: computed(() => list.find((p) => p.isActive) ?? null),
        },
      },
    },
  })
}

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
    const w = mountList({ providers: [provider('deepseek'), provider('moonshot')] })

    const rows = w.findAll('[data-provider-list] > li')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('deepseek')
    expect(rows[1].text()).toContain('moonshot')
  })

  it('行内 edit 事件带上对应的服务商', async () => {
    const w = mountList({ providers: [provider('deepseek'), provider('moonshot')] })

    await w.findAll('[data-edit]')[1].trigger('click')

    expect(w.emitted('edit')?.[0]?.[0]).toMatchObject({ serviceProvider: 'moonshot' })
  })
})

describe('ProviderList · 激活（ADR-0015）', () => {
  it('行内 activate 事件带上对应的服务商', async () => {
    const p1 = provider('deepseek')
    const p2 = provider('moonshot')
    const w = mountList({ providers: [p1, p2] })

    await w.findAll('[data-activate]')[1].trigger('click')

    expect(w.emitted('activate')?.[0]).toEqual([p2])
  })
})

describe('ProviderList · 无激活横幅（任务 18）', () => {
  const oneActive = () => [{ ...provider('deepseek'), isActive: true }, provider('relay')]

  it('没有任何服务商被激活时，列表上方出现提示（无按钮，圆点就是操作）', () => {
    const list = [provider('deepseek'), provider('relay')]
    const w = mountList({ providers: list })

    expect(w.find('[data-settings-noactive]').exists()).toBe(true)
    expect(w.text()).toContain('还没有激活的服务商')
  })

  it('有激活的服务商时不出现提示', () => {
    const w = mountList({ providers: oneActive() })

    expect(w.find('[data-settings-noactive]').exists()).toBe(false)
  })

  it('提示里没有按钮（操作就在每行的激活按钮上）', () => {
    const w = mountList({ providers: [provider('deepseek')] })

    expect(w.find('[data-settings-noactive]').exists()).toBe(true)
    expect(w.find('[data-settings-noactive] button').exists()).toBe(false)
  })
})
