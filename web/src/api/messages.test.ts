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
      ndjsonResponse(['{"type":"start","assistantMessageId":"a1","seq":2,"model":"m","serviceProvider":"p"}\n', '{"type":"delta","content":"甲"}\n', '{"type":"done","messageCount":2}\n']),
    )
    vi.stubGlobal('fetch', fetchMock)

    const events: StreamEvent[] = []
    const handle = api.send('c1', { content: '你好' }, (e) => events.push(e as StreamEvent))
    await handle.done

    expect(events.map((e) => e.type)).toEqual(['start', 'delta', 'done'])
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/v1/conversations/c1/messages')
    expect(init.method).toBe('POST')
    expect(JSON.parse(String(init.body))).toMatchObject({ content: '你好' })
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

    const handle = api.send('c1', { content: '' }, () => {})
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

    const handle = api.send('c1', { content: '你好' }, () => {})
    handle.abort()
    await expect(handle.done).resolves.toBeUndefined()
  })
})
