import { computed, ref, type Ref } from 'vue'
import * as api from '../api/conversations'
import type { Conversation } from '../api/conversations'

/** 每批条数：首屏与滚动加载共用（02-ui设计.md 流程 8） */
export const PAGE_SIZE = 10

export interface UseConversationList {
  items: Ref<Conversation[]>
  total: Ref<number>
  hasMore: Ref<boolean>
  loading: Ref<boolean>
  error: Ref<string>
  loadFirstPage: () => Promise<void>
  loadMore: () => Promise<void>
  create: () => Promise<Conversation>
  /** 本地插入一条会话（后端建的，前端没调 POST /conversations；ADR-0017 草稿态） */
  insert: (conv: Conversation) => void
  rename: (id: string, title: string) => Promise<void>
  remove: (id: string) => Promise<void>
  touch: (id: string, patch: { messageCount?: number; lastMessageTime?: string }) => void
}

/**
 * 侧栏会话列表：分页、滚动加载、增删改、本地 touch（02-前端设计.md §2.1）。
 *
 * `hasMore` 不单独存，由 `items.length < total` 派生——服务端总数是唯一真相，
 * 避免"翻到最后一页但 hasMore 还是 true"这种脏状态。
 */
export function useConversationList(): UseConversationList {
  const items = ref<Conversation[]>([])
  const total = ref(0)
  const loading = ref(false)
  const error = ref('')

  const hasMore = computed(() => items.value.length < total.value)

  async function fetchPage(page: number): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const res = await api.list({ page, pageSize: PAGE_SIZE })
      items.value = page === 1 ? res.list : [...items.value, ...res.list]
      total.value = res.total
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  function loadFirstPage(): Promise<void> {
    return fetchPage(1)
  }

  async function loadMore(): Promise<void> {
    if (loading.value || !hasMore.value) return
    await fetchPage(Math.floor(items.value.length / PAGE_SIZE) + 1)
  }

  async function create(): Promise<Conversation> {
    const created = await api.create()
    items.value = [created, ...items.value]
    total.value += 1
    return created
  }

  /** 本地插入：meta 带回新建会话时用（不请求接口）；已在列表则跳过 */
  function insert(conv: Conversation): void {
    if (items.value.some((c) => c.id === conv.id)) return
    items.value = [conv, ...items.value]
    total.value++
  }

  async function rename(id: string, title: string): Promise<void> {
    const updated = await api.rename(id, title)
    items.value = items.value.map((c) => (c.id === id ? updated : c))
  }

  async function remove(id: string): Promise<void> {
    await api.remove(id)
    items.value = items.value.filter((c) => c.id !== id)
    total.value = Math.max(0, total.value - 1)
  }

  /** 发送消息后只改本地时间与消息数，不重拉列表、不重排（避免会话在眼前跳走） */
  function touch(id: string, patch: { messageCount?: number; lastMessageTime?: string }): void {
    items.value = items.value.map((c) => (c.id === id ? { ...c, ...patch } : c))
  }

  return { items, total, hasMore, loading, error, loadFirstPage, loadMore, create, insert, rename, remove, touch }
}
