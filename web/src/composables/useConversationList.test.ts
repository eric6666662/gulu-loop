import { afterEach, describe, expect, it, vi } from 'vitest'
import { useConversationList, PAGE_SIZE } from './useConversationList'
import * as api from '../api/conversations'
import type { Conversation } from '../api/conversations'

function conv(i: number, over: Partial<Conversation> = {}): Conversation {
  return {
    id: `c${i}`,
    title: `会话 ${i}`,
    messageCount: 2,
    lastMessageTime: `2026-10-${String(i).padStart(2, '0')}T10:00:00Z`,
    createdAt: '2026-10-01T10:00:00Z',
    updatedAt: '2026-10-01T10:00:00Z',
    ...over,
  }
}

describe('useConversationList', () => {
  afterEach(() => vi.restoreAllMocks())

  it('首屏取 10 条，hasMore 由 total 决定', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({
      list: Array.from({ length: PAGE_SIZE }, (_, i) => conv(i)),
      page: 1,
      pageSize: PAGE_SIZE,
      total: 24,
    })

    const { items, hasMore, loadFirstPage } = useConversationList()
    await loadFirstPage()

    expect(items.value).toHaveLength(PAGE_SIZE)
    expect(hasMore.value).toBe(true)
    expect(api.list).toHaveBeenCalledWith({ page: 1, pageSize: PAGE_SIZE })
  })

  it('loadMore 追加下一批；取完后 hasMore 变 false', async () => {
    const first = Array.from({ length: PAGE_SIZE }, (_, i) => conv(i))
    vi.spyOn(api, 'list')
      .mockResolvedValueOnce({ list: first, page: 1, pageSize: PAGE_SIZE, total: 14 })
      .mockResolvedValueOnce({ list: [conv(10), conv(11), conv(12), conv(13)], page: 2, pageSize: PAGE_SIZE, total: 14 })

    const { items, hasMore, loadFirstPage, loadMore } = useConversationList()
    await loadFirstPage()
    await loadMore()

    expect(items.value).toHaveLength(14)
    expect(hasMore.value).toBe(false)
    expect(api.list).toHaveBeenLastCalledWith({ page: 2, pageSize: PAGE_SIZE })
  })

  it('没有更多时 loadMore 不再请求', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [conv(0)], page: 1, pageSize: PAGE_SIZE, total: 1 })

    const { loadFirstPage, loadMore } = useConversationList()
    await loadFirstPage()
    await loadMore()

    expect(api.list).toHaveBeenCalledTimes(1)
  })

  it('create 成功后插到列表最前', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [conv(1)], page: 1, pageSize: PAGE_SIZE, total: 1 })
    vi.spyOn(api, 'create').mockResolvedValue(conv(9, { title: '', messageCount: 0, lastMessageTime: null }))

    const { items, create, loadFirstPage } = useConversationList()
    await loadFirstPage()
    const created = await create()

    expect(created.id).toBe('c9')
    expect(items.value[0].id).toBe('c9')
    expect(items.value).toHaveLength(2)
  })

  it('remove 成功后从列表移除并返回被删项的下标（供选中迁移）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({
      list: [conv(1), conv(2), conv(3)],
      page: 1,
      pageSize: PAGE_SIZE,
      total: 3,
    })
    vi.spyOn(api, 'remove').mockResolvedValue(undefined)

    const { items, remove, loadFirstPage } = useConversationList()
    await loadFirstPage()
    await remove('c2')

    expect(items.value.map((c) => c.id)).toEqual(['c1', 'c3'])
  })

  it('rename 成功后就地替换标题', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({ list: [conv(1)], page: 1, pageSize: PAGE_SIZE, total: 1 })
    vi.spyOn(api, 'rename').mockResolvedValue(conv(1, { title: '新标题' }))

    const { items, rename, loadFirstPage } = useConversationList()
    await loadFirstPage()
    await rename('c1', '新标题')

    expect(items.value[0].title).toBe('新标题')
    expect(api.rename).toHaveBeenCalledWith('c1', '新标题')
  })

  it('touch 只改本地的时间与消息数（不重排、不发请求）', async () => {
    vi.spyOn(api, 'list').mockResolvedValue({
      list: [conv(1), conv(2)],
      page: 1,
      pageSize: PAGE_SIZE,
      total: 2,
    })

    const { items, touch, loadFirstPage } = useConversationList()
    await loadFirstPage()
    touch('c2', { messageCount: 9, lastMessageTime: '2026-12-01T00:00:00Z' })

    expect(items.value[1]).toMatchObject({ id: 'c2', messageCount: 9 })
    expect(items.value[0].id).toBe('c1') // 顺序不变
    expect(api.list).toHaveBeenCalledTimes(1)
  })

  it('首屏失败写 error，不抛给调用方', async () => {
    vi.spyOn(api, 'list').mockRejectedValue(new Error('boom'))

    const { items, error, loadFirstPage } = useConversationList()
    await loadFirstPage()

    expect(items.value).toEqual([])
    expect(error.value).toBe('boom')
  })
})
