import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelSwitchNotice from './ModelSwitchNotice.vue'

describe('ModelSwitchNotice', () => {
  it('主动切换：只说模型名', () => {
    const w = mount(ModelSwitchNotice, { props: { model: 'deepseek-reasoner' } })

    expect(w.text()).toBe('已切换到 deepseek-reasoner')
  })

  it('换了服务商但模型名相同时，带上服务商名（否则看不出变化）', () => {
    const w = mount(ModelSwitchNotice, { props: { model: 'deepseek-chat', provider: '中转站' } })

    expect(w.text()).toBe('已切换到 中转站 · deepseek-chat')
  })

  it('回落变体：主语是系统、原因是服务商（不是用户主动换的）', () => {
    const w = mount(ModelSwitchNotice, { props: { reason: 'fallback', provider: 'DeepSeek', model: 'deepseek-chat' } })

    expect(w.text()).toBe('服务商换成 DeepSeek，模型跟着换成 deepseek-chat')
  })
})
