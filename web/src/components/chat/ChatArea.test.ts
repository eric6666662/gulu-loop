import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ChatArea from './ChatArea.vue'
import * as api from '../../api/messages'

describe('ChatArea', () => {
  afterEach(() => vi.restoreAllMocks())

  it('草稿态（无会话 id）：显示草稿对话页 + 输入区，不请求历史', async () => {
    const list = vi.spyOn(api, 'list')

    const w = mount(ChatArea, { props: { conversationId: '', title: '' } })
    await flushPromises()

    expect(w.text()).toContain('开始新的对话')
    expect(w.find('[data-composer-textarea]').exists()).toBe(true)
    expect(list).not.toHaveBeenCalled()
  })

  it('有会话：拉历史并渲染，输入区可用', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const w = mount(ChatArea, { props: { conversationId: 'c1', title: '测试会话' } })
    await flushPromises()

    expect(w.find('h1').text()).toBe('测试会话')
    expect(w.find('[data-composer-textarea]').exists()).toBe(true)
  })
})
