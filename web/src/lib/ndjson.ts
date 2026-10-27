/**
 * NDJSON 流解析（02-前端设计.md §2.1 / ADR-0003）。
 *
 * 流式协议的唯一解析点：喂一个 Response，逐事件回调。要处理的坑全在这里——
 * 跨 chunk 半行、CRLF、空行、坏行、`done`/`error` 之后停止读取。
 */

export type StreamEvent =
  | {
      type: 'meta'
      conversationId: string
      title: string
      userMessageId: string
      assistantMessageId: string
      seq: number
      model: string
      serviceProvider: string
    }
  | { type: 'message'; content?: string; thinkingContent?: string }
  | {
      type: 'done'
      assistantMessageId?: string
      seq?: number
      messageCount?: number
      updatedAt?: string
    }
  | { type: 'error'; conversationId?: string; message: string }

const TERMINAL = new Set(['done', 'error'])

export async function readNdjson(
  res: Response,
  onEvent: (e: StreamEvent) => void,
): Promise<void> {
  const reader = res.body?.getReader()
  if (!reader) return

  const decoder = new TextDecoder()
  let buffer = ''

  const handleLine = (raw: string): boolean => {
    const line = raw.trim()
    if (!line) return true
    let event: StreamEvent
    try {
      event = JSON.parse(line) as StreamEvent
    } catch {
      return true // 坏行跳过，不中断整条流
    }
    onEvent(event)
    return !TERMINAL.has(event.type)
  }

  for (;;) {
    const { value, done } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })

    let idx: number
    while ((idx = buffer.indexOf('\n')) >= 0) {
      const line = buffer.slice(0, idx)
      buffer = buffer.slice(idx + 1)
      if (!handleLine(line)) {
        await reader.cancel().catch(() => {})
        return
      }
    }
  }

  // 结尾没有换行的最后一行
  buffer += decoder.decode()
  if (buffer.trim()) handleLine(buffer)
}
