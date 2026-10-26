import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import AppSidebar from './components/layout/AppSidebar.vue'
import ChatHeader from './components/chat/ChatHeader.vue'
import Composer from './components/chat/Composer.vue'
import MessageAssistant from './components/chat/MessageAssistant.vue'
import MessageUser from './components/chat/MessageUser.vue'
import ProviderBanner from './components/chat/ProviderBanner.vue'
import ProviderActivateButton from './components/settings/ProviderActivateButton.vue'
import ProviderRow from './components/settings/ProviderRow.vue'
import type { Message } from './api/messages'

const msg: Message = {
  id: 'm1',
  conversationId: 'c1',
  seq: 1,
  role: 'assistant',
  content: '内容',
  serviceProvider: 'deepseek',
  model: 'deepseek-v4-pro',
  thinkingContent: '',
  thinkingDuration: 0,
  createdAt: '2026-10-26T10:00:00Z',
  updatedAt: '',
}

const provider = {
  serviceProvider: 'deepseek',
  website: 'https://platform.deepseek.com',
  baseUrl: 'https://api.deepseek.com/v1',
  model: 'deepseek-v4-pro',
  models: [{ id: 'deepseek-v4-pro', name: 'V4 Pro' }],
  hasApiKey: true,
  isActive: false,
  createdAt: '2026-10-26T10:00:00Z',
  updatedAt: '2026-10-26T10:00:00Z',
}

const cases: Array<[string, () => ReturnType<typeof mount>]> = [
  [
    'AppSidebar',
    () =>
      mount(AppSidebar, {
        props: { items: [], activeId: '', hasMore: false, loading: false, open: false },
      }),
  ],
  ['ChatHeader', () => mount(ChatHeader, { props: { title: 't' } })],
  ['Composer', () => mount(Composer, { props: { streaming: false } })],
  ['MessageAssistant', () => mount(MessageAssistant, { props: { message: msg } })],
  ['MessageUser', () => mount(MessageUser, { props: { message: { ...msg, role: 'user' } } })],
  ['ProviderBanner', () => mount(ProviderBanner)],
  ['ProviderRow', () => mount(ProviderRow, { props: { provider } })],
  ['ProviderActivateButton（未激活）', () => mount(ProviderActivateButton, { props: { name: 'relay', active: false } })],
  ['ProviderActivateButton（已激活）', () => mount(ProviderActivateButton, { props: { name: 'relay', active: true } })],
]

/**
 * 键盘可达的底线：**每个 button 要么有 aria-label，要么有可见文字**。
 * 图标按钮漏 aria-label 时，屏幕阅读器只会念 "按钮"。
 */
describe('无障碍 · 图标按钮必须有可读名称', () => {
  for (const [name, make] of cases) {
    it(`${name} 里每个按钮都有 aria-label 或可见文字`, () => {
      const w = make()
      const bad: string[] = []
      for (const btn of w.findAll('button')) {
        const label = btn.attributes('aria-label')
        const text = btn.text().trim()
        if (!label && !text) bad.push(btn.html().slice(0, 80))
      }
      expect(bad).toEqual([])
    })
  }
})
