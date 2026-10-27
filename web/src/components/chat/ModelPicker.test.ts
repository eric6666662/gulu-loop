import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPicker from './ModelPicker.vue'

const models = [
  { id: 'deepseek-chat', name: 'deepseek-chat', serviceProvider: 'DeepSeek' },
  { id: 'deepseek-reasoner', name: 'deepseek-reasoner', serviceProvider: 'DeepSeek' },
]

describe('ModelPicker', () => {
  it('头部显示服务商名与生效范围，选项不带服务商副标题（v15）', () => {
    const w = mount(ModelPicker, { props: { models, modelValue: 'deepseek-chat', provider: 'DeepSeek' } })

    expect(w.text()).toContain('DeepSeek 的模型')
    expect(w.text()).toContain('这个会话的后续消息都用它')
    // 选项只有模型名，不再重复服务商
    const opts = w.findAll('[role="option"]')
    expect(opts).toHaveLength(2)
    expect(opts[0].text()).toBe('deepseek-chat')
  })

  it('点选项 emit update:modelValue 并关闭', async () => {
    const w = mount(ModelPicker, { props: { models, modelValue: 'deepseek-chat', provider: 'DeepSeek' } })

    await w.findAll('[role="option"]')[1].trigger('click')

    expect(w.emitted('update:modelValue')?.[0]).toEqual(['deepseek-reasoner'])
    expect(w.emitted('close')).toHaveLength(1)
  })

  it('当前项 aria-selected=true 且只有它一个', () => {
    const w = mount(ModelPicker, { props: { models, modelValue: 'deepseek-reasoner', provider: 'DeepSeek' } })

    const sel = w.findAll('[role="option"]').filter((o) => o.attributes('aria-selected') === 'true')
    expect(sel).toHaveLength(1)
    expect(sel[0].text()).toBe('deepseek-reasoner')
  })
})
