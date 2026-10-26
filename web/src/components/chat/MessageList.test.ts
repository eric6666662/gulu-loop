import { describe, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MessageList from './MessageList.vue'
import type { Message } from '../../api/messages'

function msg(seq: number, role: 'user' | 'assistant', content: string): Message {
  return {
    id: `m${seq}`,
    conversationId: 'c1',
    seq,
    role,
    content,
    serviceProvider: 'deepseek',
    model: 'deepseek-v4-pro',
    thinkingContent: '',
    thinkingDuration: 0,
    createdAt: '2026-10-26T10:31:00Z',
    updatedAt: '2026-10-26T10:31:00Z',
  }
}

describe('MessageList', () => {
  it('loading → 骨架屏', () => {
    const w = mount(MessageList, { props: { messages: [], status: 'loading', error: '' } })

    expect(w.find('[data-skeleton]').exists()).toBe(true)
  })

  it('空会话 → 行动邀请（不是"暂无数据"）', () => {
    const w = mount(MessageList, { props: { messages: [], status: 'idle', error: '' } })

    expect(w.text()).toContain('这个会话还是空的')
    expect(w.text()).toContain('写第一句')
  })

  it('点「写第一句」emit focus-composer', async () => {
    const w = mount(MessageList, { props: { messages: [], status: 'idle', error: '' } })

    await w.find('[data-focus-composer]').trigger('click')

    expect(w.emitted('focus-composer')).toHaveLength(1)
  })

  it('加载失败 → 错误卡 + 重试（emit retry）', async () => {
    const w = mount(MessageList, {
      props: { messages: [], status: 'error', error: '连不上本地服务' },
    })

    expect(w.text()).toContain('连不上本地服务')
    await w.find('[data-retry]').trigger('click')
    expect(w.emitted('retry')).toHaveLength(1)
  })

  it('有消息时按 seq 升序渲染，用户与助手交替出现', async () => {
    const w = mount(MessageList, {
      props: {
        messages: [msg(2, 'assistant', '回复'), msg(1, 'user', '提问')],
        status: 'idle',
        error: '',
      },
    })
    await flushPromises()

    const texts = w.findAll('[data-message]').map((m) => m.text())
    expect(texts).toHaveLength(2)
    expect(texts[0]).toContain('提问')
    expect(texts[1]).toContain('回复')
  })
})
