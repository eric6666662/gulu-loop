import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { computed, ref } from 'vue'
import { useModelSelection } from '../../composables/useModelSelection'
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
  return mountWith(activeProviders(), rest as never, __attachTo === true)
}

/** 同时提供 providers 与 modelSelection（后者是 App 层状态，见 ADR-0016 修正） */
function mountWith(providers: ReturnType<typeof activeProviders>, props: never, attachTo = false) {
  const modelSelection = useModelSelection(providers.activeProvider)
  return mount(ChatArea, {
    props,
    global: { provide: { providers, modelSelection } },
    ...(attachTo ? { attachTo: document.body } : {}),
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
    const sendSpy = vi.spyOn(api, 'send').mockImplementation((_input, onEvent) => {
      onEvent({ type: 'done', messageCount: 2 } as never)
      return { abort: () => {}, done: Promise.resolve() }
    })

    const w = mountChatArea({ conversationId: 'c1', title: 't' })
    await flushPromises()
    await w.find('[data-retry]').trigger('click')

    // regenerate：锚点是 m2 前面最近的 user（m1），送原文"第一问"
    expect(sendSpy.mock.calls[0][0]).toMatchObject({ type: 'regenerate', userMessageId: 'm1', assistantMessageId: 'm2', content: '第一问' })
  })

  it('点用户消息的「编辑」→ 弹层预填 → 保存后用新内容重发', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: thread, hasMore: false })
    const sendSpy = vi.spyOn(api, 'send').mockImplementation((_input, onEvent) => {
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

    expect(sendSpy.mock.calls[0][0]).toMatchObject({ type: 'regenerate', userMessageId: 'm1', assistantMessageId: 'm2', content: '改过的问题' })
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

// 三种空态由注入的服务商数据派生（canSend 已改为 computed，不再是 prop）
describe('ChatArea · 无服务商 / 无激活', () => {
  afterEach(() => vi.restoreAllMocks())

  it('0 个服务商 → "还没有可用的服务商"横幅 + 输入禁用', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const providers = activeProviders()
    providers.providers.value = []
    const w = mountWith(providers, { conversationId: 'c1', title: 't' } as never)
    await flushPromises()

    expect(w.text()).toContain('还没有可用的服务商')
    expect(w.find('[data-no-active]').exists()).toBe(false)
    expect(w.find('[data-composer-textarea]').attributes('disabled')).toBeDefined()
  })

  it('有服务商且有激活 → 不显示任何横幅', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const w = mountChatArea({ conversationId: 'c1', title: 't' })
    await flushPromises()

    expect(w.text()).not.toContain('还没有可用的服务商')
    expect(w.text()).not.toContain('还没有激活的服务商')
  })
})

describe('ChatArea · 模型回落痕迹（任务 17）', () => {
  afterEach(() => vi.restoreAllMocks())

  it('激活换人且模型不在新清单 → 消息区出现回落痕迹；发送后消失', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    vi.spyOn(api, 'send').mockImplementation((_input, onEvent) => {
      onEvent({ type: 'done', messageCount: 2 } as never)
      return { abort: () => {}, done: Promise.resolve() }
    })

    const providers = activeProviders()
    const w = mountWith(providers, { conversationId: 'c1', title: 't', canSend: true } as never)
    await flushPromises()

    // 切到另一个服务商，且它的清单里没有当前模型 → 回落
    providers.providers.value = [
      {
        serviceProvider: 'relay',
        baseUrl: 'https://relay.example.com/v1',
        model: 'gpt-4o',
        models: [{ id: 'gpt-4o', name: 'GPT-4o' }],
        hasApiKey: true,
        isActive: true,
        createdAt: '2026-10-26T00:00:00Z',
        updatedAt: '2026-10-26T00:00:00Z',
      },
    ]
    await flushPromises()

    expect(w.text()).toContain('服务商换成 relay，模型跟着换成 gpt-4o')

    // 发一条 → 痕迹只留到下一轮
    await w.find('[data-composer-textarea]').setValue('你好')
    await w.find('[data-composer-textarea]').trigger('keydown', { key: 'Enter' })
    await flushPromises()

    expect(w.text()).not.toContain('服务商换成 relay')
  })
})

describe('ChatArea · 有服务商但没有激活（任务 18）', () => {
  afterEach(() => vi.restoreAllMocks())

  it('有服务商无激活：显示"没有激活"横幅，且输入区与发送禁用', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const providers = activeProviders()
    providers.providers.value = [{ ...providers.providers.value[0]!, isActive: false }]
    const w = mountWith(providers, { conversationId: 'c1', title: 't' } as never)
    await flushPromises()

    expect(w.text()).toContain('还没有激活的服务商')
    expect(w.find('[data-no-active]').exists()).toBe(true)
    expect(w.find('[data-composer-textarea]').attributes('disabled')).toBeDefined()
    expect(w.find('[data-primary]').attributes('disabled')).toBeDefined()
  })

  it('没有服务商：只显示"还没有可用的服务商"横幅（两横幅互斥）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const providers = activeProviders()
    providers.providers.value = []
    const w = mountWith(providers, { conversationId: 'c1', title: 't' } as never)
    await flushPromises()

    expect(w.text()).toContain('还没有可用的服务商')
    expect(w.find('[data-no-active]').exists()).toBe(false)
    expect(w.find('[data-composer-textarea]').attributes('disabled')).toBeDefined()
  })

  it('有激活的服务商：两个横幅都不出现，输入区可用', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const w = mountChatArea({ conversationId: 'c1', title: 't' })
    await flushPromises()

    expect(w.text()).not.toContain('还没有可用的服务商')
    expect(w.text()).not.toContain('还没有激活的服务商')
    expect(w.find('[data-composer-textarea]').attributes('disabled')).toBeUndefined()
  })

  it('点「去选择」→ 打开服务商设置', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })

    const providers = activeProviders()
    providers.providers.value = [{ ...providers.providers.value[0]!, isActive: false }]
    const w = mountWith(providers, { conversationId: 'c1', title: 't' } as never)
    await flushPromises()

    await w.find('[data-no-active]').find('[data-open-settings-list]').trigger('click')

    expect(w.emitted('open-settings')).toHaveLength(1)
  })
})
