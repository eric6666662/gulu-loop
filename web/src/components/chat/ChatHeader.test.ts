import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ChatHeader from './ChatHeader.vue'

describe('ChatHeader', () => {
  it('渲染标题（空标题显示「新的会话」）', () => {
    expect(mount(ChatHeader, { props: { title: '快速排序' } }).find('h1').text()).toBe('快速排序')
    expect(mount(ChatHeader, { props: { title: '' } }).find('h1').text()).toBe('新的会话')
  })

  it('刷新 / 编辑 / 分享三个按钮各自 emit', async () => {
    const w = mount(ChatHeader, { props: { title: 't' } })

    await w.find('[data-refresh]').trigger('click')
    await w.find('[data-rename]').trigger('click')
    await w.find('[data-export]').trigger('click')

    expect(w.emitted('refresh')).toHaveLength(1)
    expect(w.emitted('rename')).toHaveLength(1)
    expect(w.emitted('export')).toHaveLength(1)
  })
})
