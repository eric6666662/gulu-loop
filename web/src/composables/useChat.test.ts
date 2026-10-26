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
