import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './client'
import * as api from './conversations'

function stubFetch(body: unknown, status = 200) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () =>
      new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
    ),
  )
}

describe('api/conversations.exportMarkdown', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('返回 {filename, content}（JSON，不是文件流）', async () => {
    stubFetch({ code: 200, message: 'ok', data: { filename: '会话.md', content: '# 标题' } })

    const res = await api.exportMarkdown('c1')

    expect(res).toEqual({ filename: '会话.md', content: '# 标题' })
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0]
    expect(url).toBe('/api/v1/conversations/c1/export')
  })

  it('404 抛 ApiError', async () => {
    stubFetch({ code: 404, message: '会话不存在' }, 404)

    await expect(api.exportMarkdown('nope')).rejects.toBeInstanceOf(ApiError)
  })
})
