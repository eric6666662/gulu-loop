import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelSwitchButton from './ModelSwitchButton.vue'

const models = [
  { id: 'deepseek-v4-pro', name: 'V4 Pro', serviceProvider: 'deepseek' },
  { id: 'kimi-k2', name: 'kimi-k2', serviceProvider: 'moonshot' },
]

describe('ModelSwitchButton + ModelPicker', () => {
  it('默认只显示按钮，弹层关闭', () => {
    const w = mount(ModelSwitchButton, { props: { models, modelValue: 'deepseek-v4-pro' } })

    expect(w.find('[data-model-picker]').exists()).toBe(false)
    expect(w.find('[data-model-switch]').attributes('aria-expanded')).toBe('false')
  })

  it('按钮显示当前模型名，点开弹层后 aria-expanded=true', async () => {
    const w = mount(ModelSwitchButton, { props: { models, modelValue: 'deepseek-v4-pro' } })
    expect(w.find('[data-model-name]').text()).toBe('deepseek-v4-pro')

    await w.find('[data-model-switch]').trigger('click')

    expect(w.find('[data-model-picker]').exists()).toBe(true)
    expect(w.find('[data-model-switch]').attributes('aria-expanded')).toBe('true')
  })

  it('选项列出所有模型，带所属服务商；当前项打勾', async () => {
    const w = mount(ModelSwitchButton, { props: { models, modelValue: 'deepseek-v4-pro' } })
    await w.find('[data-model-switch]').trigger('click')

    const opts = w.findAll('[data-model-option]')
    expect(opts).toHaveLength(2)
    expect(opts[0].text()).toContain('deepseek')
    expect(opts[0].attributes('aria-selected')).toBe('true')
    expect(opts[1].attributes('aria-selected')).toBe('false')
  })

  it('选中后 emit update:modelValue 并关闭弹层', async () => {
    const w = mount(ModelSwitchButton, { props: { models, modelValue: 'deepseek-v4-pro' } })
    await w.find('[data-model-switch]').trigger('click')

    await w.findAll('[data-model-option]')[1].trigger('click')

    expect(w.emitted('update:modelValue')?.[0]?.[0]).toBe('kimi-k2')
    expect(w.find('[data-model-picker]').exists()).toBe(false)
  })

  it('Esc 关闭弹层', async () => {
    const w = mount(ModelSwitchButton, { props: { models, modelValue: 'deepseek-v4-pro' }, attachTo: document.body })
    await w.find('[data-model-switch]').trigger('click')

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()

    expect(w.find('[data-model-picker]').exists()).toBe(false)
    w.unmount()
  })

  it('无服务商（models 为空）时按钮禁用、弹层打不开', async () => {
    const w = mount(ModelSwitchButton, { props: { models: [], modelValue: '', disabled: true } })

    expect(w.find('[data-model-switch]').attributes('disabled')).toBeDefined()
    await w.find('[data-model-switch]').trigger('click')
    expect(w.find('[data-model-picker]').exists()).toBe(false)
  })
})
