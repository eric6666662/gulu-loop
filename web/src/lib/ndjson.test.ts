import { describe, expect, it } from 'vitest'
import { readNdjson, type StreamEvent } from './ndjson'

/** 把若干片段拼成一个假 Response（模拟网络分块到达） */
function fakeResponse(chunks: string[]): Response {
  const enc = new TextEncoder()
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c))
      controller.close()
    },
  })
  return new Response(stream, { headers: { 'Content-Type': 'application/x-ndjson' } })
}

async function collect(chunks: string[]): Promise<StreamEvent[]> {
  const got: StreamEvent[] = []
  await readNdjson(fakeResponse(chunks), (e) => got.push(e))
  return got
}

describe('lib/ndjson', () => {
  it('一行一个事件', async () => {
    const got = await collect(['{"type":"delta","content":"甲"}\n{"type":"done","messageCount":2}\n'])

    expect(got.map((e) => e.type)).toEqual(['delta', 'done'])
    expect(got[0]).toMatchObject({ type: 'delta', content: '甲' })
  })

  it('一条 JSON 被切成两个 chunk 仍能拼回', async () => {
    const got = await collect(['{"type":"delta","con', 'tent":"半行"}\n'])

    expect(got).toHaveLength(1)
    expect(got[0]).toMatchObject({ type: 'delta', content: '半行' })
  })

  it('结尾没有换行的最后一行也算', async () => {
    const got = await collect(['{"type":"done","messageCount":1}'])

    expect(got).toHaveLength(1)
    expect(got[0].type).toBe('done')
  })

  it('CRLF 与空行都能处理', async () => {
    const got = await collect(['{"type":"delta","content":"甲"}\r\n\r\n{"type":"done"}\r\n'])

    expect(got.map((e) => e.type)).toEqual(['delta', 'done'])
  })

  it('坏行跳过、不中断后续事件', async () => {
    const got = await collect(['not json\n{"type":"delta","content":"好的"}\n'])

    expect(got).toHaveLength(1)
    expect(got[0]).toMatchObject({ type: 'delta', content: '好的' })
  })

  it('done 之后不再回调（即使后面还有数据）', async () => {
    const got = await collect(['{"type":"done"}\n{"type":"delta","content":"不该出现"}\n'])

    expect(got.map((e) => e.type)).toEqual(['done'])
  })

  it('error 事件把 message 透传出来，并停止读取', async () => {
    const got = await collect(['{"type":"error","message":"调用超时"}\n{"type":"delta","content":"x"}\n'])

    expect(got).toHaveLength(1)
    expect(got[0].type).toBe('error')
    expect(got[0]).toMatchObject({ type: 'error', message: '调用超时' })
  })
})
