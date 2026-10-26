import { request } from './client'

/** 会话（02-后端设计.md §1.2） */
export interface Conversation {
  id: string
  title: string
  messageCount: number
  lastMessageTime: string | null
  createdAt: string
  updatedAt: string
}

export interface Page<T> {
  list: T[]
  page: number
  pageSize: number
  total: number
}

export function list(params: { page?: number; pageSize?: number } = {}): Promise<Page<Conversation>> {
  const q = new URLSearchParams()
  if (params.page) q.set('page', String(params.page))
  if (params.pageSize) q.set('pageSize', String(params.pageSize))
  const qs = q.toString()
  return request<Page<Conversation>>(`/api/v1/conversations${qs ? `?${qs}` : ''}`)
}

export function create(): Promise<Conversation> {
  return request<Conversation>('/api/v1/conversations', { method: 'POST' })
}

export function rename(id: string, title: string): Promise<Conversation> {
  return request<Conversation>(`/api/v1/conversations/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify({ title }),
  })
}

export function remove(id: string): Promise<void> {
  return request<void>(`/api/v1/conversations/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

/** 导出会话（#9）：返回 JSON，前端自己造 Blob 下载 */
export interface ExportResult {
  filename: string
  content: string
}

export function exportMarkdown(id: string): Promise<ExportResult> {
  return request<ExportResult>(`/api/v1/conversations/${encodeURIComponent(id)}/export`)
}
