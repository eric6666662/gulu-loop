import { describe, expect, it } from 'vitest'
import { computed, ref } from 'vue'
import { useModelSelection } from './useModelSelection'
import type { Provider } from '../api/providers'

function provider(name: string, model: string, ids: string[], isActive = false): Provider {
  return {
    serviceProvider: name,
    baseUrl: `https://${name}.com/v1`,
    model,
    models: ids.map((id) => ({ id, name: id })),
    hasApiKey: true,
    isActive,
    createdAt: '2026-10-26T00:00:00Z',
    updatedAt: '2026-10-26T00:00:00Z',
  }
}

describe('useModelSelection（ADR-0016）', () => {
  it('初值 = 激活服务商的默认模型', () => {
    const active = ref<Provider | null>(provider('deepseek', 'deepseek-chat', ['deepseek-chat', 'deepseek-reasoner'], true))

    const s = useModelSelection(computed(() => active.value))

    expect(s.selected.value).toEqual({ serviceProvider: 'deepseek', model: 'deepseek-chat' })
    expect(s.label.value).toBe('deepseek-chat')
  })

  it('select 改值', () => {
    const active = ref<Provider | null>(provider('deepseek', 'deepseek-chat', ['deepseek-chat', 'deepseek-reasoner'], true))
    const s = useModelSelection(computed(() => active.value))

    s.select({ serviceProvider: 'deepseek', model: 'deepseek-reasoner' })

    expect(s.selected.value?.model).toBe('deepseek-reasoner')
  })

  it('激活换人且当前模型不在新清单 → 回落到新服务商的默认模型，并记 fallback', async () => {
    const active = ref<Provider | null>(provider('deepseek', 'deepseek-chat', ['deepseek-chat', 'deepseek-reasoner'], true))
    const s = useModelSelection(computed(() => active.value))
    s.select({ serviceProvider: 'deepseek', model: 'deepseek-reasoner' })

    active.value = provider('relay', 'gpt-4o', ['gpt-4o'], true)
    await Promise.resolve()

    expect(s.selected.value).toEqual({ serviceProvider: 'relay', model: 'gpt-4o' })
    expect(s.fallback.value).toEqual({ from: 'deepseek-reasoner', to: 'gpt-4o' })
  })

  it('激活换人但当前模型也在新清单 → 不回落、serviceProvider 跟着变、fallback 为空', async () => {
    const active = ref<Provider | null>(provider('official', 'deepseek-chat', ['deepseek-chat'], true))
    const s = useModelSelection(computed(() => active.value))

    active.value = provider('relay', 'deepseek-chat', ['deepseek-chat', 'gpt-4o'], true)
    await Promise.resolve()

    expect(s.selected.value).toEqual({ serviceProvider: 'relay', model: 'deepseek-chat' })
    expect(s.fallback.value).toBeNull()
  })

  it('无激活 → selected 为 null、models 为空', () => {
    const active = ref<Provider | null>(null)

    const s = useModelSelection(computed(() => active.value))

    expect(s.selected.value).toBeNull()
    expect(s.models.value).toEqual([])
    expect(s.label.value).toBe('')
  })

  it('clearFallback 清空痕迹', async () => {
    const active = ref<Provider | null>(provider('a', 'm1', ['m1'], true))
    const s = useModelSelection(computed(() => active.value))
    active.value = provider('b', 'm2', ['m2'], true)
    await Promise.resolve()
    expect(s.fallback.value).not.toBeNull()

    s.clearFallback()

    expect(s.fallback.value).toBeNull()
  })

  it('同名模型：models 只含激活服务商的，selected 带正确的 serviceProvider', () => {
    const active = ref<Provider | null>(provider('official', 'deepseek-chat', ['deepseek-chat'], true))

    const s = useModelSelection(computed(() => active.value))

    expect(s.models.value).toEqual([{ id: 'deepseek-chat', name: 'deepseek-chat', serviceProvider: 'official' }])
    expect(s.selected.value?.serviceProvider).toBe('official')
  })
})
