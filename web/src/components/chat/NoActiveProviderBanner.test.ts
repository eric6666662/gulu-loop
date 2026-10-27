import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import NoActiveProviderBanner from './NoActiveProviderBanner.vue'

describe('NoActiveProviderBanner', () => {
  it('文案说清"缺什么 + 怎么补"，不出现禁用词', () => {
    const w = mount(NoActiveProviderBanner)

    expect(w.text()).toContain('还没有激活的服务商')
    expect(w.text()).toContain('去服务商设置点一下行尾的 ⏻')
    expect(w.text()).not.toContain('暂无数据')
  })

  it('点「去选择」emit open-settings', async () => {
    const w = mount(NoActiveProviderBanner)

    await w.find('[data-open-settings-list]').trigger('click')

    expect(w.emitted('open-settings')).toHaveLength(1)
  })
})
