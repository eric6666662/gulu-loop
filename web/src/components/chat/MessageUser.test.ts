import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MessageUser from './MessageUser.vue'
import type { Message } from '../../api/messages'

const msg: Message = {
  id: 'm1',
  conversationId: 'c1',
  seq: 1,
  role: 'user',
  content: '帮我把快排改成泛型',
  serviceProvider: '',
  model: '',
  thinkingContent: '',
  thinkingDuration: 0,
  createdAt: '2026-10-26T10:00:00Z',
  updatedAt: '2026-10-26T10:00:00Z',
}

describe('MessageUser', () => {
  it('渲染内容，右对齐气泡', () => {
    const w = mount(MessageUser, { props: { message: msg } })

    expect(w.text()).toContain('帮我把快排改成泛型')
    expect(w.find('.rounded-xl').exists()).toBe(true)
  })

  it('hover 操作组里有「编辑」，点击 emit edit', async () => {
    const w = mount(MessageUser, { props: { message: msg } })

    const btn = w.find('[data-edit]')
    expect(btn.attributes('aria-label')).toBe('编辑这条消息')
    await btn.trigger('click')

    expect(w.emitted('edit')).toHaveLength(1)
  })
})

describe('MessageUser · 复制', () => {
  it('点「复制」emit copy', async () => {
    const w = mount(MessageUser, { props: { message: msg } })

    await w.find('[data-copy]').trigger('click')

    expect(w.emitted('copy')).toHaveLength(1)
  })
})
