import { ApiError, request } from './client'
import { readNdjson, type StreamEvent } from '../lib/ndjson'

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

export interface SendInput {
  /** send = 正常发送；regenerate = 重试/编辑后重发（两者在后端同一条路径） */
  type: 'send' | 'regenerate'
  /** send 可选（无则后端新建会话，草稿态）；regenerate 必填 */
  conversationId?: string
  content: string
  /** 服务商标识：必须等于当前激活的服务商（ADR-0015） */
  serviceProvider?: string
  model?: string
  /** regenerate 的截断锚点（ADR-0017）：从它的 seq 起全删 */
  userMessageId?: string
  /** regenerate 的配对校验（可空）：编辑最后一条时它可能还没有 assistant 回复 */
  assistantMessageId?: string
}

export interface SendHandle {
  abort: () => void
  done: Promise<void>
}

/**
 * 发送消息（流式）：开流前的错误抛 ApiError（JSON 信封），开流后的事件走 onEvent。
 *
 * 返回 { abort, done }：`abort()` 中断本次流；`done` 在流结束（含 abort）后 resolve，
 * 开流前失败则 reject。调用方不需要自己读 ReadableStream。
 */
export function send(
  input: SendInput,
  onEvent: (e: StreamEvent) => void,
): SendHandle {
  const controller = new AbortController()

  const done = (async () => {
    let res: Response
    try {
      // conversationId 在 body 里（可选，无则后端新建会话；ADR-0017）
      res = await fetch('/api/v1/messages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
        signal: controller.signal,
      })
    } catch (e) {
      if (e instanceof Error && e.name === 'AbortError') return // 用户主动停止，不算失败
      throw new ApiError(0, '连不上本地服务，检查 127.0.0.1:1112 是否还在跑')
    }

    if (!res.ok) {
      // 开流前的错误一律是 JSON 信封（§1.3）
      const body = await res.json().catch(() => null)
      const message =
        (body as { message?: string } | null)?.message ?? `请求失败：HTTP ${res.status}`
      throw new ApiError(res.status, message)
    }

    try {
      await readNdjson(res, onEvent)
    } catch (e) {
      if (e instanceof Error && e.name === 'AbortError') return
      throw e
    }
  })()

  return { abort: () => controller.abort(), done }
}

/** 单条消息的纯文本内容（#8 复制） */
export async function content(convId: string, messageId: string): Promise<string> {
  const data = await request<{ content: string }>(
    `/api/v1/conversations/${encodeURIComponent(convId)}/messages/${encodeURIComponent(messageId)}`,
  )
  return data.content
}
