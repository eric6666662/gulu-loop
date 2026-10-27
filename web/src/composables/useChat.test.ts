import { afterEach, describe, expect, it, vi } from 'vitest'
import { ref, nextTick } from 'vue'
import { useChat } from './useChat'
import * as api from '../api/messages'
import type { Message } from '../api/messages'

function msg(seq: number, content: string): Message {
  return {
    id: `m${seq}`,
    conversationId: 'c1',
    seq,
    role: 'assistant',
    content,
    serviceProvider: 'deepseek',
    model: 'deepseek-v4-pro',
    thinkingContent: '',
    thinkingDuration: 0,
    createdAt: '2026-10-26T10:00:00Z',
    updatedAt: '2026-10-26T10:00:00Z',
  }
}

describe('useChat', () => {
  afterEach(() => vi.restoreAllMocks())

  it('进会话即拉历史，成功后 status=idle', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [msg(2, '二'), msg(1, '一')], hasMore: false })

    const id = ref('c1')
    const { messages, status } = useChat(id)
    await vi.waitFor(() => expect(status.value).toBe('idle'))

    expect(messages.value).toHaveLength(2)
    expect(api.list).toHaveBeenCalledWith('c1', { limit: 50 })
  })

  it('拉取失败 → status=error 且带 message', async () => {
    vi.spyOn(api, 'list').mockRejectedValue(new Error('连不上本地服务'))

    const id = ref('c1')
    const { status, error } = useChat(id)
    await vi.waitFor(() => expect(status.value).toBe('error'))

    expect(error.value).toBe('连不上本地服务')
  })

  it('切会话时丢弃过期响应（先发的慢请求不能覆盖后发的）', async () => {
    let resolveFirst: (v: { list: Message[]; hasMore: boolean }) => void = () => {}
    vi.spyOn(api, 'list')
      .mockImplementationOnce(
        () => new Promise((r) => { resolveFirst = r as never }),
      )
      .mockResolvedValueOnce({ list: [msg(9, '新会话的消息')], hasMore: false })

    const id = ref('c1')
    const { messages, status } = useChat(id)
    await nextTick()

    id.value = 'c2' // 切会话
    await vi.waitFor(() => expect(status.value).toBe('idle'))
    expect(messages.value[0].content).toBe('新会话的消息')

    // 旧请求这时才回来 —— 必须被丢弃
    resolveFirst({ list: [msg(1, '旧会话的消息')], hasMore: false })
    await new Promise((r) => setTimeout(r, 10))
    expect(messages.value[0].content).toBe('新会话的消息')
  })

  it('空会话 id → 清空消息，不发请求', async () => {
    const spy = vi.spyOn(api, 'list')

    const id = ref('')
    const { messages, status } = useChat(id)
    await nextTick()

    expect(messages.value).toEqual([])
    expect(status.value).toBe('idle')
    expect(spy).not.toHaveBeenCalled()
  })

  it('refresh 可手动重拉', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [msg(1, '一')], hasMore: false })

    const id = ref('c1')
    const { messages, refresh } = useChat(id)
    await vi.waitFor(() => expect(messages.value).toHaveLength(1))

    await refresh()
    expect(api.list).toHaveBeenCalledTimes(2)
  })
})

// ---------- 发送（任务 6） ----------
import * as msgApi from '../api/messages'

function deltaChunk(content: string) {
  return { type: 'delta' as const, content }
}

function fakeSend(events: Array<Record<string, unknown>>, opts: { failBefore?: Error } = {}) {
  return vi.spyOn(msgApi, 'send').mockImplementation((_id, _input, onEvent) => {
    if (opts.failBefore) {
      return { abort: () => {}, done: Promise.reject(opts.failBefore) }
    }
    const done = (async () => {
      for (const e of events) onEvent(e as never)
    })()
    return { abort: () => {}, done }
  })
}

describe('useChat.send', () => {
  afterEach(() => vi.restoreAllMocks())

  it('乐观插入用户消息 → streaming 占位 → delta 追加 → done 收尾', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    fakeSend([
      { type: 'start', assistantMessageId: 'a1', seq: 2, model: 'deepseek-v4-pro', serviceProvider: 'deepseek' },
      deltaChunk('泛型版'),
      deltaChunk('参数化。'),
      { type: 'done', messageCount: 2 },
    ])
    const onDone = vi.fn()

    const id = ref('c1')
    const { messages, status, send } = useChat(id, { onDone })
    await send('帮我改成泛型')

    expect(messages.value.map((m) => m.role)).toEqual(['user', 'assistant'])
    expect(messages.value[0].content).toBe('帮我改成泛型')
    expect(messages.value[1].content).toBe('泛型版参数化。')
    expect(messages.value[1].model).toBe('deepseek-v4-pro')
    expect(status.value).toBe('idle')
    expect(onDone).toHaveBeenCalledWith('c1', 2)
  })

  it('error 事件 → status=error，保留已收内容', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    fakeSend([
      { type: 'start', assistantMessageId: 'a1', seq: 2, model: 'm', serviceProvider: 'p' },
      deltaChunk('收到一半'),
      { type: 'error', message: '调用超时' },
    ])

    const { messages, status, error, send } = useChat(ref('c1'))
    await send('你好')

    expect(status.value).toBe('error')
    expect(error.value).toBe('调用超时')
    expect(messages.value[1].content).toBe('收到一半')
  })

  it('开流前失败（ApiError）→ status=error，不插入 assistant 占位', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    fakeSend([], { failBefore: new Error('该会话正在生成回复') })

    const { messages, status, error, send } = useChat(ref('c1'))
    await send('你好')

    expect(status.value).toBe('error')
    expect(error.value).toBe('该会话正在生成回复')
    expect(messages.value.filter((m) => m.role === 'assistant')).toHaveLength(0)
  })

  it('草稿态（无会话）：先建会话，通知 App 选中，再用新 id 发送', async () => {
    const ensureConversation = vi.fn(async () => 'new-c1')
    const onConversationReady = vi.fn()
    const sendSpy = fakeSend([{ type: 'done', messageCount: 1 }])

    const { send } = useChat(ref(''), { ensureConversation, onConversationReady })
    await send('第一句')

    expect(ensureConversation).toHaveBeenCalledTimes(1)
    expect(onConversationReady).toHaveBeenCalledWith('new-c1')
    expect(sendSpy.mock.calls[0][0]).toBe('new-c1')
  })

  it('空内容不发送', async () => {
    const sendSpy = fakeSend([])
    const { send } = useChat(ref('c1'))
    await send('   ')
    expect(sendSpy).not.toHaveBeenCalled()
  })

  it('stop() 中断并保留已收内容', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    let abortFn: (() => void) | null = null
    vi.spyOn(msgApi, 'send').mockImplementation((_id, _input, onEvent) => {
      onEvent({ type: 'start', assistantMessageId: 'a1', seq: 2, model: 'm', serviceProvider: 'p' } as never)
      onEvent({ type: 'delta', content: '一半' } as never)
      return { abort: () => { abortFn = () => {} }, done: new Promise<void>(() => {}) }
    })

    const { messages, status, send, stop } = useChat(ref('c1'))
    void send('你好')
    await vi.waitFor(() => expect(status.value).toBe('streaming'))

    stop()

    expect(abortFn).not.toBeNull()
    expect(status.value).toBe('idle')
    expect(messages.value[1].content).toBe('一半')
  })
})

describe('useChat 竞态', () => {
  afterEach(() => vi.restoreAllMocks())

  it('历史比发送晚回来时，不覆盖乐观插入的消息', async () => {
    let resolveList: (v: { list: Message[]; hasMore: boolean }) => void = () => {}
    vi.spyOn(api, 'list').mockImplementation(
      () => new Promise((r) => { resolveList = r as never }),
    )
    fakeSend([
      { type: 'start', assistantMessageId: 'a1', seq: 2, model: 'm', serviceProvider: 'p' },
      { type: 'done', messageCount: 2 },
    ])

    const { messages, send } = useChat(ref('c1'))
    await send('你好')

    // 历史这时才回来（空列表）——不能把刚发的两条消息冲掉
    resolveList({ list: [], hasMore: false })
    await new Promise((r) => setTimeout(r, 10))

    expect(messages.value.map((m) => m.role)).toEqual(['user', 'assistant'])
  })
})

describe('useChat · 错误来源区分', () => {
  afterEach(() => vi.restoreAllMocks())

  it('生成中失败：errorKind=stream，且空占位被摘掉', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    fakeSend([
      { type: 'start', assistantMessageId: 'a1', seq: 2, model: 'm', serviceProvider: 'p' },
      { type: 'error', message: '上游返回 401：api_key 无效' },
    ])

    const { messages, status, error, errorKind, send } = useChat(ref('c1'))
    await send('你好')

    expect(status.value).toBe('error')
    expect(errorKind.value).toBe('stream')
    expect(error.value).toContain('401')
    expect(messages.value.filter((m) => m.role === 'assistant')).toHaveLength(0) // 空占位已摘
    expect(messages.value.filter((m) => m.role === 'user')).toHaveLength(1)
  })

  it('拉历史失败：errorKind=load', async () => {
    vi.spyOn(api, 'list').mockRejectedValue(new Error('连不上本地服务'))

    const { status, errorKind } = useChat(ref('c1'))
    await vi.waitFor(() => expect(status.value).toBe('error'))

    expect(errorKind.value).toBe('load')
  })
})

describe('useChat.retry / edit', () => {
  afterEach(() => vi.restoreAllMocks())

  const thread: Message[] = [
    { ...msg(1, '第一问'), id: 'm1', role: 'user' },
    { ...msg(2, '第一答'), id: 'm2', role: 'assistant' },
    { ...msg(3, '第二问'), id: 'm3', role: 'user' },
    { ...msg(4, '第二答'), id: 'm4', role: 'assistant' },
  ]

  function mountWithThread() {
    vi.spyOn(api, 'list').mockResolvedValue({ list: thread, hasMore: false })
    return useChat(ref('c1'))
  }

  it('retry：截断该 assistant 及其后，带 messageId 重发', async () => {
    const { messages, retry } = mountWithThread()
    await vi.waitFor(() => expect(messages.value).toHaveLength(4))

    const spy = fakeSend([
      { type: 'start', assistantMessageId: 'm4-new', seq: 4, model: 'm', serviceProvider: 'p' },
      deltaChunk('重答'),
      { type: 'done', messageCount: 4 },
    ])
    await retry('m4')

    expect(spy.mock.calls[0][1]).toMatchObject({ messageId: 'm4' })
    expect(messages.value.map((m) => m.id)).toEqual(['m1', 'm2', 'm3', 'm4-new'])
    expect(messages.value[3].content).toBe('重答')
  })

  it('edit：截断该 user 及其后，乐观插入新内容并带 messageId', async () => {
    const { messages, edit } = mountWithThread()
    await vi.waitFor(() => expect(messages.value).toHaveLength(4))

    const spy = fakeSend([
      { type: 'start', assistantMessageId: 'a-new', seq: 4, model: 'm', serviceProvider: 'p' },
      { type: 'done', messageCount: 4 },
    ])
    await edit('m3', '改过的第二问')

    expect(spy.mock.calls[0][1]).toMatchObject({ messageId: 'm3', content: '改过的第二问' })
    const contents = messages.value.map((m) => m.content)
    expect(contents).toContain('改过的第二问')
    expect(contents).not.toContain('第二问')
    expect(contents).not.toContain('第二答')
  })

  it('retry 一个不存在的消息 → 不发请求', async () => {
    const { retry } = mountWithThread()
    const spy = fakeSend([])

    await retry('ghost')

    expect(spy).not.toHaveBeenCalled()
  })
})

describe('useChat · 乐观消息的服务端化', () => {
  afterEach(() => vi.restoreAllMocks())

  it('start 事件到达后，乐观 user 消息换成服务端 id 与 seq（排序与重试才不会错）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    fakeSend([
      { type: 'start', userMessageId: 'u-real', assistantMessageId: 'a1', seq: 2, model: 'm', serviceProvider: 'p' },
      { type: 'done', messageCount: 2 },
    ])

    const { messages, send } = useChat(ref('c1'))
    await send('你好')

    const user = messages.value.find((m) => m.role === 'user')
    expect(user?.id).toBe('u-real')
    expect(user?.seq).toBe(1)
    // 按 seq 排序时 user 在 assistant 之前
    const ordered = [...messages.value].sort((a, b) => a.seq - b.seq)
    expect(ordered.map((m) => m.role)).toEqual(['user', 'assistant'])
  })
})

describe('useChat · 模型切换', () => {
  afterEach(() => vi.restoreAllMocks())

  it('send 带上 getModel 给的 (serviceProvider, model)（ADR-0016）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    const spy = fakeSend([{ type: 'done', messageCount: 1 }])

    const { send } = useChat(ref('c1'), {
      getModel: () => ({ serviceProvider: 'relay', model: 'kimi-k2' }),
    })

    await send('你好')

    expect(spy.mock.calls[0][1]).toMatchObject({ serviceProvider: 'relay', model: 'kimi-k2' })
  })

  it('getModel 返回 null 时请求体不带这两个字段（用激活服务商的默认模型）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [], hasMore: false })
    const spy = fakeSend([{ type: 'done', messageCount: 1 }])

    const { send } = useChat(ref('c1'), { getModel: () => null })
    await send('你好')

    const body = spy.mock.calls[0][1] as { model?: string; serviceProvider?: string }
    expect(body.model).toBeUndefined()
    expect(body.serviceProvider).toBeUndefined()
  })

  it('retry 与 edit 也带当前模型（重试不读原消息快照）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({
      list: [
        { id: 'u1', conversationId: 'c1', seq: 1, role: 'user', content: 'q', model: '', serviceProvider: '', thinkingContent: '', thinkingDuration: 0, createdAt: '', updatedAt: '' },
        { id: 'a1', conversationId: 'c1', seq: 2, role: 'assistant', content: 'a', model: 'old-model', serviceProvider: 'official', thinkingContent: '', thinkingDuration: 0, createdAt: '', updatedAt: '' },
      ],
      hasMore: false,
    })
    const spy = fakeSend([{ type: 'done', messageCount: 2 }])

    const { retry, edit, refresh } = useChat(ref('c1'), {
      getModel: () => ({ serviceProvider: 'relay', model: 'new-model' }),
    })
    await refresh()

    await retry('a1')
    expect(spy.mock.calls[0][1]).toMatchObject({ serviceProvider: 'relay', model: 'new-model' })

    await edit('u1', '改过的')
    expect(spy.mock.calls[1][1]).toMatchObject({ serviceProvider: 'relay', model: 'new-model' })
  })
})
