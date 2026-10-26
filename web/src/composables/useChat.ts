import { ref, watch, type Ref } from 'vue'
import * as api from '../api/messages'
import type { Message } from '../api/messages'

export type ChatStatus = 'idle' | 'loading' | 'error'

export interface UseChat {
  messages: Ref<Message[]>
  status: Ref<ChatStatus>
  error: Ref<string>
  refresh: () => Promise<void>
}

/** 一次拉多少条历史（向上翻页在任务 6 之后按需接） */
const HISTORY_LIMIT = 50

/**
 * 聊天区唯一的深模块（02-前端设计.md §2.1）。
 *
 * 任务 5 只落"拉历史 + 三种态"；发送 / 停止 / 重试 / 编辑在任务 6、7 接进来，
 * 组件始终只调这里暴露的方法。
 */
export function useChat(conversationId: Ref<string>): UseChat {
  const messages = ref<Message[]>([])
  const status = ref<ChatStatus>('idle')
  const error = ref('')

  // 请求序号：切会话时旧请求可能后到，用它丢弃过期响应
  let ticket = 0

  async function refresh(): Promise<void> {
    const id = conversationId.value
    if (!id) {
      messages.value = []
      status.value = 'idle'
      error.value = ''
      return
    }

    const mine = ++ticket
    status.value = 'loading'
    error.value = ''
    try {
      const page = await api.list(id, { limit: HISTORY_LIMIT })
      if (mine !== ticket) return // 已切到别的会话
      messages.value = page.list
      status.value = 'idle'
    } catch (e) {
      if (mine !== ticket) return
      error.value = e instanceof Error ? e.message : String(e)
      status.value = 'error'
    }
  }

  watch(conversationId, refresh, { immediate: true })

  return { messages, status, error, refresh }
}
