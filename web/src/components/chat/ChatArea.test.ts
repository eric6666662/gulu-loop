import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import ChatArea from './ChatArea.vue'
import * as api from '../../api/messages'
import * as conversationApi from '../../api/conversations'
import type { Provider } from '../../api/providers'

/**
 * ChatArea 从 App provide 下来的服务商实例读激活项（02-前端设计.md §3），
 * 所以每个用例都要给一份假 providers。
 */
function activeProviders(over: Partial<Provider> = {}) {
  const p: Provider = {
    serviceProvider: 'deepseek',
    baseUrl: 'https://api.deepseek.com/v1',
    model: 'deepseek-v4-pro',
    models: [
      { id: 'deepseek-v4-pro', name: 'V4 Pro' },
      { id: 'kimi-k2', name: 'Kimi K2' },
    ],
    hasApiKey: true,
    isActive: true,
    createdAt: '2026-10-26T00:00:00Z',
    updatedAt: '2026-10-26T00:00:00Z',
    ...over,
  }
  const providers = ref<Provider[]>([p])
  return {
    providers,
    activeProvider: computed(() => providers.value.find((x) => x.isActive) ?? null),
    models: computed(() => []),
    loading: ref(false),
    error: ref(''),
    load: vi.fn(),
    activate: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    remove: vi.fn(),
    test: vi.fn(),
  }
}

function mountChatArea(props: Record<string, unknown>) {
  const { __attachTo, ...rest } = props as { __attachTo?: boolean }
  return mount(ChatArea, {
    props: rest as never,
    global: { provide: { providers: activeProviders() } },
    ...(__attachTo ? { attachTo: document.body } : {}),
  })
}

describe('ChatArea', () => {
  afterEach(() => vi.restoreAllMocks())

  it('草稿态（无会话 id）：显示草稿对话页 + 输入区，不请求历史', async () => {
    const list = vi.spyOn(api, 'list')

    const w = mountChatArea({ conversationId: '', title: '' })
    await flushPromises()

    expect(w.text()).toContain('开始新的对话')
    expect(w.find('[data-composer-textarea]').exists()).toBe(true)
    expect(list).not.toHaveBeenCalled()
  })

  it('有会话：拉历史并渲染，输入区可用', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const w = mountChatArea({ conversationId: 'c1', title: '测试会话' })
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

    const w = mountChatArea({ conversationId: 'c1', title: 't' })
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

    const w = mountChatArea({ ...{ conversationId: 'c1', title: 't' }, __attachTo: true })
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

describe('ChatArea · 导出与复制', () => {
  afterEach(() => vi.restoreAllMocks())

  const thread = [
    { id: 'm1', conversationId: 'c1', seq: 1, role: 'user', content: '第一问', serviceProvider: '', model: '', thinkingContent: '', thinkingDuration: 0, createdAt: '2026-10-26T10:00:00Z', updatedAt: '' },
    { id: 'm2', conversationId: 'c1', seq: 2, role: 'assistant', content: '第一答', serviceProvider: 'deepseek', model: 'deepseek-v4-pro', thinkingContent: '', thinkingDuration: 0, createdAt: '2026-10-26T10:01:00Z', updatedAt: '' },
  ] as never

  it('点分享 → 调导出接口（拿 JSON 造 Blob）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: thread, hasMore: false })
    const exportSpy = vi
      .spyOn(conversationApi, 'exportMarkdown')
      .mockResolvedValue({ filename: '会话.md', content: '# 标题' })

    const w = mountChatArea({ ...{ conversationId: 'c1', title: 't' }, __attachTo: true })
    await flushPromises()
    await w.find('[data-export]').trigger('click')
    await flushPromises()

    expect(exportSpy).toHaveBeenCalledWith('c1')
    w.unmount()
  })

  it('点助手消息的「复制」→ 调单条内容接口', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: thread, hasMore: false })
    const contentSpy = vi.spyOn(api, 'content').mockResolvedValue('第一答')

    const w = mountChatArea({ ...{ conversationId: 'c1', title: 't' }, __attachTo: true })
    await flushPromises()
    await w.findAll('[data-copy]')[1].trigger('click') // 第二条 = assistant
    await flushPromises()

    expect(contentSpy).toHaveBeenCalledWith('c1', 'm2')
    w.unmount()
  })
})

describe('ChatArea · 无服务商', () => {
  afterEach(() => vi.restoreAllMocks())

  it('没有可用服务商时显示横幅，且输入区禁用', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const w = mountChatArea({ conversationId: 'c1', title: 't', canSend: false })
    await flushPromises()

    expect(w.text()).toContain('还没有可用的服务商')
    expect(w.find('[data-composer-textarea]').attributes('disabled')).toBeDefined()
  })

  it('有服务商时不显示横幅', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const w = mountChatArea({ conversationId: 'c1', title: 't', canSend: true })
    await flushPromises()

    expect(w.text()).not.toContain('还没有可用的服务商')
  })
})
