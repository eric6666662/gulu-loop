import { request } from './client'

/** 模型清单元素（02-后端设计.md §1.2） */
export interface ModelRef {
  id: string
  name: string
}

/** 服务商视图：api_key 永不回传，只给 hasApiKey */
export interface Provider {
  serviceProvider: string
  website?: string
  baseUrl: string
  model: string
  models: ModelRef[]
  hasApiKey: boolean
  createdAt: string
  updatedAt: string
}

/** 新增 / 更新请求体（camelCase，02-后端设计.md §1.6） */
export interface ProviderInput {
  serviceProvider: string
  website: string
  baseUrl: string
  apiKey: string
  model: string
  models: ModelRef[]
}

export function list(): Promise<Provider[]> {
  return request<Provider[]>('/api/v1/service-providers')
}

export function create(input: ProviderInput): Promise<Provider> {
  return request<Provider>('/api/v1/service-providers', {
    method: 'POST',
    body: JSON.stringify(input),
  })
}

export function update(name: string, input: ProviderInput): Promise<Provider> {
  return request<Provider>(`/api/v1/service-providers/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  })
}

export function remove(name: string): Promise<void> {
  return request<void>(`/api/v1/service-providers/${encodeURIComponent(name)}`, { method: 'DELETE' })
}
