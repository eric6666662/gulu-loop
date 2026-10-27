import { ref, watch, type Ref } from 'vue'
import * as api from '../api/messages'
import type { Message, SendHandle } from '../api/messages'

export type ChatStatus = 'idle' | 'loading' | 'streaming' | 'error'

/** 当前选中的模型身份（ADR-0016）：由 useModelSelection 提供，useChat 不自己持有 */
export interface ChatModelRef {
  serviceProvider: string
  model: string
}

export interface ChatDeps {
  /** meta 带回新会话（草稿态 send 后端建的）：App 用来 select + 插侧栏（带标题） */
  onConversationReady?: (id: string, title: string) => void
  /** 一轮结束：App 用来 touch 列表（消息数 / 时间） */
  onDone?: (conversationId: string, messageCount: number) => void
  /**
   * 每次请求前读当前选中的 (serviceProvider, model)。
   * 返回 null 表示不带这两个字段（后端用激活服务商的默认模型）。
   */
  getModel?: () => ChatModelRef | null
}

export interface UseChat {
  messages: Ref<Message[]>
  status: Ref<ChatStatus>
  error: Ref<string>
  /** 错误来源：load=拉历史失败；stream=生成中失败（两者文案不同） */
  errorKind: Ref<'load' | 'stream' | ''>
  refresh: () => Promise<void>
  send: (text: string) => Promise<void>
  /** 重试：regenerate，锚点是这条 assistant 前面最近的 user 消息（ADR-0017） */
  retry: (messageId: string) => Promise<void>
  /** 编辑后重发：regenerate，锚点是这条 user 消息，用新内容（ADR-0017） */
  edit: (messageId: string, content: string) => Promise<void>
  stop: () => void
}

/** 一次拉多少条历史（向上翻页在任务 6 之后按需接） */
const HISTORY_LIMIT = 50

/**
 * 聊天区唯一的深模块（02-前端设计.md §2.1）。
 *
 * 任务 5 只落"拉历史 + 三种态"；发送 / 停止 / 重试 / 编辑在任务 6、7 接进来，
 * 组件始终只调这里暴露的方法。
 */
export function useChat(conversationId: Ref<string>, deps: ChatDeps = {}): UseChat {
  const messages = ref<Message[]>([])
  const status = ref<ChatStatus>('idle')
  const error = ref('')
  const errorKind = ref<'load' | 'stream' | ''>('')

  // 请求序号：切会话时旧请求可能后到，用它丢弃过期响应
  let ticket = 0
  // 在途的流式请求（stop 用）
  let handle: SendHandle | null = null

  async function refresh(): Promise<void> {
    const id = conversationId.value
    if (!id) {
      messages.value = []
      status.value = 'idle'
      error.value = ''
      return
    }

    const mine = ++ticket
    // 发送中不要被"加载历史"盖掉状态
    if (status.value !== 'streaming') status.value = 'loading'
    error.value = ''
    try {
      const page = await api.list(id, { limit: HISTORY_LIMIT })
      if (mine !== ticket) return // 已切到别的会话
      // 历史比发送晚回来时，不能把乐观插入的消息覆盖掉（竞态）
      if (status.value === 'streaming') return
      messages.value = page.list
      status.value = 'idle'
    } catch (e) {
      if (mine !== ticket) return
      error.value = e instanceof Error ? e.message : String(e)
      errorKind.value = 'load'
      status.value = 'error'
    }
  }

  watch(conversationId, refresh, { immediate: true })

  function blankMessage(over: Partial<Message>): Message {
    return {
      id: '',
      conversationId: '',
      seq: 0,
      role: 'assistant',
      content: '',
      serviceProvider: '',
      model: '',
      thinkingContent: '',
      thinkingDuration: 0,
      createdAt: new Date().toISOString(),
      updatedAt: '',
      ...over,
    }
  }

  /**
   * 一次流式往返的公共骨架：send / retry / edit 都走这里（ADR-0017）。
   *
   * `input.conversationId` 可为空（草稿态 send，后端建会话）；meta 事件带回新会话 id 与标题。
   */
  async function runStream(input: api.SendInput, optimisticUser?: string): Promise<void> {
    error.value = ''
    errorKind.value = ''
    status.value = 'streaming'
    // 作废任何在途的"加载历史"请求：从这一刻起消息列表由本次发送接管
    ticket++

    let activeConvId = input.conversationId ?? ''
    if (optimisticUser !== undefined) {
      messages.value = [
        ...messages.value,
        blankMessage({ id: `local-${Date.now()}`, conversationId: activeConvId, role: 'user', content: optimisticUser }),
      ]
    }

    let assistantId = ''
    // 每次请求都读当前选中的 (serviceProvider, model)：重试/编辑也用**当前**选择
    const sel = deps.getModel?.() ?? null
    const payload = sel ? { ...input, serviceProvider: sel.serviceProvider, model: sel.model } : input
    handle = api.send(payload, (e) => {
      switch (e.type) {
        case 'meta': {
          assistantId = e.assistantMessageId
          activeConvId = e.conversationId
          // 把乐观插入的 user 消息换成服务端 id / seq（regenerate 时是**新** id）
          const local = [...messages.value].reverse().find((m) => m.id.startsWith('local-'))
          if (local && e.userMessageId) {
            messages.value = messages.value.map((m) =>
              m.id === local.id
                ? { ...m, id: e.userMessageId, conversationId: e.conversationId, seq: e.seq - 1 }
                : m,
            )
          }
          messages.value = [
            ...messages.value,
            blankMessage({
              id: assistantId,
              conversationId: e.conversationId,
              seq: e.seq,
              role: 'assistant',
              model: e.model,
              serviceProvider: e.serviceProvider,
            }),
          ]
          // 草稿态 send：meta 带回新会话 → 通知 App select + 插侧栏（带标题）
          if (e.conversationId && e.conversationId !== conversationId.value) {
            deps.onConversationReady?.(e.conversationId, e.title)
          }
          break
        }
        case 'message': {
          if (e.thinkingContent) {
            messages.value = messages.value.map((m) =>
              m.id === assistantId ? { ...m, thinkingContent: m.thinkingContent + e.thinkingContent } : m,
            )
          }
          if (e.content) {
            messages.value = messages.value.map((m) =>
              m.id === assistantId ? { ...m, content: m.content + e.content } : m,
            )
          }
          break
        }
        case 'done':
          status.value = 'idle'
          deps.onDone?.(activeConvId, e.messageCount ?? 0)
          break
        case 'error': {
          // 已经收到的内容保留；一个字都没收到就把空占位摘掉
          messages.value = messages.value.filter((m) => !(m.id === assistantId && m.content === ''))
          error.value = e.message
          errorKind.value = 'stream'
          status.value = 'error'
          break
        }
      }
    })

    try {
      await handle.done
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      errorKind.value = 'stream'
      status.value = 'error'
    } finally {
      handle = null
      if (status.value === 'streaming') status.value = 'idle'
    }
  }

  async function send(text: string): Promise<void> {
    const content = text.trim()
    if (!content || status.value === 'streaming') return

    // 草稿态（conversationId 空）：不带 conversationId，后端建会话（ADR-0017）
    await runStream(
      { type: 'send', content, conversationId: conversationId.value || undefined },
      content,
    )
  }

  /** 重试：regenerate，锚点是这条 assistant 前面最近的 user 消息（送原文） */
  async function retry(messageId: string): Promise<void> {
    const convId = conversationId.value
    if (!convId || status.value === 'streaming') return

    const anchor = messages.value.find((m) => m.id === messageId)
    if (!anchor || anchor.role !== 'assistant') return
    const userMsg = [...messages.value].filter((m) => m.role === 'user' && m.seq < anchor.seq).pop()
    if (!userMsg) return

    // 本地先截断（服务端也会截断）：从 user 消息起删掉，再乐观插回它
    messages.value = messages.value.filter((m) => m.seq < userMsg.seq)
    await runStream(
      {
        type: 'regenerate',
        conversationId: convId,
        content: userMsg.content,
        userMessageId: userMsg.id,
        assistantMessageId: anchor.id,
      },
      userMsg.content,
    )
  }

  /** 编辑后重发：regenerate，锚点是这条 user 消息，用新内容（ADR-0017） */
  async function edit(messageId: string, content: string): Promise<void> {
    const convId = conversationId.value
    const text = content.trim()
    if (!convId || !text || status.value === 'streaming') return

    const anchor = messages.value.find((m) => m.id === messageId)
    if (!anchor || anchor.role !== 'user') return
    const asstMsg = [...messages.value].filter((m) => m.role === 'assistant' && m.seq > anchor.seq).shift()

    messages.value = messages.value.filter((m) => m.seq < anchor.seq)
    await runStream(
      {
        type: 'regenerate',
        conversationId: convId,
        content: text,
        userMessageId: anchor.id,
        assistantMessageId: asstMsg?.id,
      },
      text,
    )
  }

  function stop(): void {
    handle?.abort()
    handle = null
    if (status.value === 'streaming') status.value = 'idle'
  }

  /** 会话内切换模型：后续消息都用它（01-需求 #9） */
  return {
    messages,
    status,
    error,
    errorKind,
    refresh,
    send,
    retry,
    edit,
    stop,
  }
}
