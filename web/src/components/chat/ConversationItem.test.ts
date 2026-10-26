import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ConversationItem from './ConversationItem.vue'
import type { Conversation } from '../../api/conversations'

const conv: Conversation = {
  id: 'c1',
  title: '快速排序的 Go 泛型版本',
  messageCount: 18,
  lastMessageTime: '2026-10-26T10:31:00Z',
  createdAt: '2026-10-20T10:00:00Z',
  updatedAt: '2026-10-26T10:31:00Z',
}

describe('ConversationItem', () => {
  it('渲染标题、消息数、相对时间', () => {
    const w = mount(ConversationItem, { props: { conversation: conv, active: false } })

    expect(w.text()).toContain('快速排序的 Go 泛型版本')
    expect(w.text()).toContain('18 条消息')
    // 相对时间四档之一：HH:MM / 昨天 / 周X / MM-DD
    expect(w.find('time').text()).toMatch(/^(\d{2}:\d{2}|昨天|周[日一二三四五六]|\d{2}-\d{2})$/)
  })

  it('当前项带 aria-current', () => {
    const w = mount(ConversationItem, { props: { conversation: conv, active: true } })

    expect(w.find('[data-conv]').attributes('aria-current')).toBe('true')
  })

  it('点行 emit select；点 × 只 emit delete（不触发 select）', async () => {
    const w = mount(ConversationItem, { props: { conversation: conv, active: false } })

    await w.find('[data-conv]').trigger('click')
    await w.find('[data-conv-delete]').trigger('click')

    expect(w.emitted('select')).toHaveLength(1)
    expect(w.emitted('delete')).toHaveLength(1)
  })

  it('标题为空时显示占位「新会话」', () => {
    const w = mount(ConversationItem, {
      props: { conversation: { ...conv, title: '' }, active: false },
    })

    expect(w.text()).toContain('新会话')
  })
})
