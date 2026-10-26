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

describe('ChatArea · 重试与编辑', () => {
  afterEach(() => vi.restoreAllMocks())

  const thread = [
    { id: 'm1', conversationId: 'c1', seq: 1, role: 'user', content: '第一问', serviceProvider: '', model: '', thinkingContent: '', thinkingDuration: 0, createdAt: '2026-10-26T10:00:00Z', updatedAt: '' },
    { id: 'm2', conversationId: 'c1', seq: 2, role: 'assistant', content: '第一答', serviceProvider: 'deepseek', model: 'deepseek-v4-pro', thinkingContent: '', thinkingDuration: 0, createdAt: '2026-10-26T10:01:00Z', updatedAt: '' },
  ] as never

  it('点助手消息的「重试」→ 用 messageId 重发', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: thread, hasMore: false })
    const sendSpy = vi.spyOn(api, 'send').mockImplementation((_id, _input, onEvent) => {
      onEvent({ type: 'done', messageCount: 2 } as never)
      return { abort: () => {}, done: Promise.resolve() }
    })

    const w = mount(ChatArea, { props: { conversationId: 'c1', title: 't' } })
    await flushPromises()
    await w.find('[data-retry]').trigger('click')

    expect(sendSpy.mock.calls[0][1]).toMatchObject({ messageId: 'm2' })
  })

  it('点用户消息的「编辑」→ 弹层预填 → 保存后用新内容重发', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: thread, hasMore: false })
    const sendSpy = vi.spyOn(api, 'send').mockImplementation((_id, _input, onEvent) => {
      onEvent({ type: 'done', messageCount: 2 } as never)
      return { abort: () => {}, done: Promise.resolve() }
    })

    const w = mount(ChatArea, { props: { conversationId: 'c1', title: 't' }, attachTo: document.body })
    await flushPromises()
    await w.find('[data-edit]').trigger('click')
    await w.vm.$nextTick()

    const dialog = w.find('[role="dialog"]')
    expect((dialog.find('[data-content-input]').element as HTMLTextAreaElement).value).toBe('第一问')

    await dialog.find('[data-content-input]').setValue('改过的问题')
    await dialog.find('[data-save]').trigger('click')

    expect(sendSpy.mock.calls[0][1]).toMatchObject({ messageId: 'm1', content: '改过的问题' })
    w.unmount()
  })
})
