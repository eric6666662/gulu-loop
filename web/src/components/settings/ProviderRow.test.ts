import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderRow from './ProviderRow.vue'
import type { Provider } from '../../api/providers'

const provider: Provider = {
  serviceProvider: 'DeepSeek',
  website: 'https://platform.deepseek.com',
  baseUrl: 'https://api.deepseek.com/v1',
  model: 'deepseek-v4-pro',
  models: [
    { id: 'deepseek-v4-flash', name: 'V4 Flash' },
    { id: 'deepseek-v4-pro', name: 'V4 Pro' },
    { id: 'deepseek-reasoner', name: 'R1' },
  ],
  hasApiKey: true,
  createdAt: '2026-10-26T00:00:00Z',
  updatedAt: '2026-10-26T00:00:00Z',
}

describe('ProviderRow', () => {
  it('渲染名称、模型数、官网域名（去掉协议头）', () => {
    const w = mount(ProviderRow, { props: { provider } })

    expect(w.text()).toContain('DeepSeek')
    expect(w.text()).toContain('3 个模型')
    expect(w.text()).toContain('platform.deepseek.com')
    expect(w.text()).not.toContain('https://')
  })

  it('website 为空时不渲染官网链接行', () => {
    const w = mount(ProviderRow, { props: { provider: { ...provider, website: '' } } })

    expect(w.find('[data-website]').exists()).toBe(false)
  })

  it('点编辑按钮 emit edit', async () => {
    const w = mount(ProviderRow, { props: { provider } })

    await w.find('[data-edit]').trigger('click')

    expect(w.emitted('edit')).toHaveLength(1)
  })

  it('不渲染「使用中」徽标或「使用」按钮（01-需求 #1f 已撤销，防回归）', () => {
    const w = mount(ProviderRow, { props: { provider } })

    expect(w.text()).not.toContain('使用中')
    expect(w.find('[data-use-provider]').exists()).toBe(false)
  })

  it('点 ⋯ 展开菜单，再点收起', async () => {
    const w = mount(ProviderRow, { props: { provider }, attachTo: document.body })

    expect(w.find('[role="menu"]').exists()).toBe(false)
    await w.find('[data-menu]').trigger('click')
    expect(w.find('[role="menu"]').exists()).toBe(true)
    expect(w.find('[data-menu]').attributes('aria-expanded')).toBe('true')

    await w.find('[data-menu]').trigger('click')
    expect(w.find('[role="menu"]').exists()).toBe(false)
    w.unmount()
  })

  it('菜单里「检测连通」emit test、「删除」emit remove，并收起菜单', async () => {
    const w = mount(ProviderRow, { props: { provider }, attachTo: document.body })

    await w.find('[data-menu]').trigger('click')
    await w.find('[data-menu-test]').trigger('click')
    expect(w.emitted('test')).toHaveLength(1)
    expect(w.find('[role="menu"]').exists()).toBe(false)

    await w.find('[data-menu]').trigger('click')
    await w.find('[data-menu-remove]').trigger('click')
    expect(w.emitted('remove')).toHaveLength(1)
    w.unmount()
  })

  it('Esc 关闭菜单', async () => {
    const w = mount(ProviderRow, { props: { provider }, attachTo: document.body })

    await w.find('[data-menu]').trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()

    expect(w.find('[role="menu"]').exists()).toBe(false)
    w.unmount()
  })

  it('编辑按钮有可读的 aria-label', () => {
    const w = mount(ProviderRow, { props: { provider } })

    expect(w.find('[data-edit]').attributes('aria-label')).toBe('编辑 DeepSeek')
  })
})
