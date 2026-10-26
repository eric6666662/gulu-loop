import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import MessageAssistant from './MessageAssistant.vue'
import type { Message } from '../../api/messages'

const msg: Message = {
  id: 'm1',
  conversationId: 'c1',
  seq: 2,
  role: 'assistant',
  content: '**粗体** 与 `code`\n\n```go\npackage main\n```',
  serviceProvider: 'deepseek',
  model: 'deepseek-v4-pro',
  thinkingContent: '',
  thinkingDuration: 0,
  createdAt: '2026-10-26T10:31:00Z',
  updatedAt: '2026-10-26T10:31:00Z',
}

describe('MessageAssistant', () => {
  it('正文按 Markdown 渲染（无气泡容器）', async () => {
    const w = mount(MessageAssistant, { props: { message: msg } })

    // 首次渲染要等 shiki 异步初始化，用 waitFor 而不是一次性断言
    await vi.waitFor(() => {
      expect(w.find('[data-body]').html()).toContain('<strong>粗体</strong>')
    })
    const body = w.find('[data-body]')
    expect(body.html()).toContain('<code class="rounded-xs bg-surface-alt')
    expect(body.html()).toContain('data-copy-code')
  })

  it('落款行常驻：模型名 + 时间', async () => {
    const w = mount(MessageAssistant, { props: { message: msg } })
    await flushPromises()

    expect(w.find('[data-footer]').text()).toContain('deepseek-v4-pro')
    expect(w.find('[data-footer] time').exists()).toBe(true)
  })

  it('思考内容不在正文里渲染（MVP 不展示）', async () => {
    const w = mount(MessageAssistant, {
      props: { message: { ...msg, thinkingContent: '让我想想…' } },
    })
    await flushPromises()

    expect(w.text()).not.toContain('让我想想')
  })
})

describe('MessageAssistant · 操作组', () => {
  it('hover 操作组里有「重试」，点击 emit retry', async () => {
    const w = mount(MessageAssistant, { props: { message: msg } })
    await flushPromises()

    const btn = w.find('[data-retry]')
    expect(btn.attributes('aria-label')).toBe('重试这条回复')
    await btn.trigger('click')

    expect(w.emitted('retry')).toHaveLength(1)
  })
})

describe('MessageAssistant · 复制', () => {
  it('点「复制」emit copy', async () => {
    const w = mount(MessageAssistant, { props: { message: msg } })
    await flushPromises()

    await w.find('[data-copy]').trigger('click')

    expect(w.emitted('copy')).toHaveLength(1)
  })
})
