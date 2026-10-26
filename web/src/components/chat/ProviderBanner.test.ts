import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderBanner from './ProviderBanner.vue'

describe('ProviderBanner', () => {
  it('文案是行动邀请，不是"暂无数据"', () => {
    const w = mount(ProviderBanner)

    expect(w.text()).toContain('还没有可用的服务商')
    expect(w.text()).toContain('才能发送消息')
    expect(w.text()).not.toContain('暂无数据')
  })

  it('点「添加服务商」emit open-settings', async () => {
    const w = mount(ProviderBanner)

    await w.find('[data-open-settings]').trigger('click')

    expect(w.emitted('open-settings')).toHaveLength(1)
  })
})
