import { ref, watch, type Ref } from 'vue'
import * as api from '../api/messages'
import type { Message, SendHandle } from '../api/messages'

export type ChatStatus = 'idle' | 'loading' | 'streaming' | 'error'

export interface ChatDeps {
  /** 草稿态首次发送：先建会话（App 注入，内部走 POST /conversations） */
  ensureConversation?: () => Promise<string>
  /** 会话刚建好：App 用来 select + 插到侧栏最前 */
  onConversationReady?: (id: string) => void
  /** 一轮结束：App 用来 touch 列表（消息数 / 时间） */
  onDone?: (conversationId: string, messageCount: number) => void
}

export interface UseChat {
  messages: Ref<Message[]>
  status: Ref<ChatStatus>
  error: Ref<string>
  /** 错误来源：load=拉历史失败；stream=生成中失败（两者文案不同） */
  errorKind: Ref<'load' | 'stream' | ''>
  refresh: () => Promise<void>
  send: (text: string) => Promise<void>
  /** 重试：截断这条 assistant 及其后，用前面最近的 user 内容重发（ADR-0006） */
  retry: (messageId: string) => Promise<void>
  /** 编辑后重发：截断这条 user 及其后，用新内容重发（ADR-0006） */
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

  /** 一次流式往返的公共骨架：send / retry / edit 都走这里 */
  async function runStream(
    convId: string,
    input: api.SendInput,
    optimisticUser?: string,
  ): Promise<void> {
    error.value = ''
    errorKind.value = ''
    status.value = 'streaming'
    // 作废任何在途的"加载历史"请求：从这一刻起消息列表由本次发送接管，
    // 否则历史晚回来会把乐观插入的消息整片覆盖掉（竞态）
    ticket++

    if (optimisticUser !== undefined) {
      // 乐观插入用户消息（前端先看见自己说的话）
      messages.value = [
        ...messages.value,
        blankMessage({ id: `local-${Date.now()}`, conversationId: convId, role: 'user', content: optimisticUser }),
      ]
    }

    let assistantId = ''
    handle = api.send(convId, input, (e) => {
      switch (e.type) {
        case 'start': {
          assistantId = e.assistantMessageId
          // 把乐观插入的 user 消息换成服务端 id / seq：
          // 否则它带着 `local-*` 与 seq=0，排序会跑到最前、重试/编辑也会拿错 id
          const local = [...messages.value].reverse().find((m) => m.id.startsWith('local-'))
          if (local && e.userMessageId) {
            messages.value = messages.value.map((m) =>
              m.id === local.id ? { ...m, id: e.userMessageId as string, seq: e.seq - 1 } : m,
            )
          }
          messages.value = [
            ...messages.value,
            blankMessage({
              id: assistantId,
              conversationId: convId,
              seq: e.seq,
              role: 'assistant',
              model: e.model,
              serviceProvider: e.serviceProvider,
            }),
          ]
          break
        }
        case 'delta':
          messages.value = messages.value.map((m) =>
            m.id === assistantId ? { ...m, content: m.content + e.content } : m,
          )
          break
        case 'done':
          status.value = 'idle'
          deps.onDone?.(convId, e.messageCount ?? 0)
          break
        case 'error': {
          // 已经收到的内容保留；一个字都没收到就把空占位摘掉（"这次回复没有写入会话"）
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

    let convId = conversationId.value
    if (!convId) {
      // 草稿态：先建会话（01-需求 #5b），建好通知 App 选中并插入侧栏
      if (!deps.ensureConversation) return
      try {
        convId = await deps.ensureConversation()
      } catch (e) {
        error.value = e instanceof Error ? e.message : String(e)
        errorKind.value = 'stream'
        status.value = 'error'
        return
      }
      deps.onConversationReady?.(convId)
    }

    await runStream(convId, { content }, content)
  }

  /** 重试：截断这条 assistant 及其后，用前面最近的 user 内容重发（ADR-0006） */
  async function retry(messageId: string): Promise<void> {
    const convId = conversationId.value
    if (!convId || status.value === 'streaming') return

    const anchor = messages.value.find((m) => m.id === messageId)
    if (!anchor) return

    // 本地先截断（服务端也会截断），让 UI 立刻回到"待生成"状态
    messages.value = messages.value.filter((m) => m.seq < anchor.seq)
    await runStream(convId, { messageId })
  }

  /** 编辑后重发：截断这条 user 及其后，用新内容重发（ADR-0006） */
  async function edit(messageId: string, content: string): Promise<void> {
    const convId = conversationId.value
    const text = content.trim()
    if (!convId || !text || status.value === 'streaming') return

    const anchor = messages.value.find((m) => m.id === messageId)
    if (!anchor) return

    messages.value = messages.value.filter((m) => m.seq < anchor.seq)
    await runStream(convId, { messageId, content: text }, text)
  }

  /** 停止生成：中断流，已收到的内容保留 */
  function stop(): void {
    handle?.abort()
    handle = null
    if (status.value === 'streaming') status.value = 'idle'
  }

  return { messages, status, error, errorKind, refresh, send, retry, edit, stop }
}
