import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './client'
import * as api from './messages'
import type { StreamEvent } from '../lib/ndjson'

function ndjsonResponse(lines: string[]): Response {
  const enc = new TextEncoder()
  const stream = new ReadableStream<Uint8Array>({
    start(c) {
      for (const l of lines) c.enqueue(enc.encode(l))
      c.close()
    },
  })
  return new Response(stream, { status: 200, headers: { 'Content-Type': 'application/x-ndjson' } })
}

describe('api/messages.send', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('POST 到会话消息接口，逐事件回调，done 后 resolve', async () => {
    const fetchMock = vi.fn(async () =>
      ndjsonResponse(['{"type":"meta","conversationId":"c1","title":"t","userMessageId":"u1","assistantMessageId":"a1","seq":2,"model":"m","serviceProvider":"p"}\n', '{"type":"message","content":"甲"}\n', '{"type":"done","messageCount":2}\n']),
    )
    vi.stubGlobal('fetch', fetchMock)

    const events: StreamEvent[] = []
    const handle = api.send({ type: 'send', conversationId: 'c1', content: '你好' }, (e) => events.push(e as StreamEvent))
    await handle.done

    expect(events.map((e) => e.type)).toEqual(['meta', 'message', 'done'])
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/messages')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toMatchObject({ type: 'send', conversationId: 'c1', content: '你好' })
  })

  it('开流前的 4xx 抛 ApiError（message 可直出）', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify({ code: 400, message: 'content 不能为空' }), {
          status: 400,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )

    const handle = api.send({ type: 'send', conversationId: 'c1', content: '' }, () => {})
    await expect(handle.done).rejects.toBeInstanceOf(ApiError)
  })

  it('abort() 后 done 不会挂起（AbortError 被吞掉）', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        return new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => {
            reject(Object.assign(new Error('aborted'), { name: 'AbortError' }))
          })
        })
      }),
    )

    const handle = api.send({ type: 'send', conversationId: 'c1', content: '你好' }, () => {})
    handle.abort()
    await expect(handle.done).resolves.toBeUndefined()
  })
})

describe('api/messages.content', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('GET 单条消息并取出 content', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        new Response(JSON.stringify({ code: 200, message: 'ok', data: { content: '纯文本' } }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )

    const text = await api.content('c1', 'm1')

    expect(text).toBe('纯文本')
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(url).toBe('/api/v1/conversations/c1/messages/m1')
  })
})

describe('send · 新契约（ADR-0017）', () => {
  it('POST 到 /api/v1/messages，conversationId/type 在 body', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(`{"type":"meta","conversationId":"c1","title":"t","userMessageId":"u1","assistantMessageId":"a1","seq":2,"model":"m","serviceProvider":"p"}\n`, { status: 200 }),
    )
    vi.stubGlobal('fetch', fetchMock)

    api.send({ type: 'send', content: 'hi', conversationId: 'c1' }, () => {})
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled())

    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/messages')
    expect(JSON.parse(init.body)).toMatchObject({ type: 'send', conversationId: 'c1', content: 'hi' })
  })

  it('regenerate 带 userMessageId + assistantMessageId', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    api.send({ type: 'regenerate', conversationId: 'c1', content: '改', userMessageId: 'u1', assistantMessageId: 'a1' }, () => {})
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled())

    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toMatchObject({
      type: 'regenerate', userMessageId: 'u1', assistantMessageId: 'a1',
    })
  })
})
