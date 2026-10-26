import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderActivateButton from './ProviderActivateButton.vue'

describe('ProviderActivateButton', () => {
  it('已激活：渲染文字「激活」+ aria-pressed=true', () => {
    const w = mount(ProviderActivateButton, { props: { name: 'DeepSeek', active: true } })

    expect(w.text()).toContain('激活')
    expect(w.attributes('aria-pressed')).toBe('true')
    expect(w.attributes('aria-label')).toBe('已激活 DeepSeek')
  })

  it('未激活：不渲染文字（纯图标）+ aria-pressed=false', () => {
    const w = mount(ProviderActivateButton, { props: { name: 'Moonshot Kimi', active: false } })

    expect(w.find('[data-active-label]').exists()).toBe(false)
    expect(w.attributes('aria-pressed')).toBe('false')
    expect(w.attributes('aria-label')).toBe('激活 Moonshot Kimi')
  })

  it('点未激活的 emit activate', async () => {
    const w = mount(ProviderActivateButton, { props: { name: 'relay', active: false } })

    await w.find('button').trigger('click')

    expect(w.emitted('activate')).toHaveLength(1)
  })

  it('点已激活的不 emit（幂等，避免无意义请求）', async () => {
    const w = mount(ProviderActivateButton, { props: { name: 'relay', active: true } })

    await w.find('button').trigger('click')

    expect(w.emitted('activate')).toBeUndefined()
  })
})
