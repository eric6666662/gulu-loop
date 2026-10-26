import { request } from './client'

export type Role = 'system' | 'user' | 'assistant' | 'tool'

/** 消息（02-后端设计.md §1.2） */
export interface Message {
  id: string
  conversationId: string
  seq: number
  role: Role
  content: string
  serviceProvider: string
  model: string
  thinkingContent: string
  thinkingDuration: number
  createdAt: string
  updatedAt: string
}

export interface MessagePage {
  list: Message[]
  hasMore: boolean
}

/** 会话内消息：**后端按 seq 倒序返回**，前端渲染前要按 seq 升序排 */
export function list(
  convId: string,
  params: { limit?: number; beforeSeq?: number } = {},
): Promise<MessagePage> {
  const q = new URLSearchParams()
  if (params.limit) q.set('limit', String(params.limit))
  if (params.beforeSeq) q.set('beforeSeq', String(params.beforeSeq))
  const qs = q.toString()
  return request<MessagePage>(
    `/api/v1/conversations/${encodeURIComponent(convId)}/messages${qs ? `?${qs}` : ''}`,
  )
}
