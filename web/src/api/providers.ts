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
  /** 是否为「激活的服务商」：全局恰好 0 或 1 个为 true（ADR-0015） */
  isActive: boolean
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

/** 检测连通结果（#15） */
export interface ProviderTestResult {
  ok: boolean
  latencyMs: number
  modelCount: number
}

/** 拉取模型列表请求（#14）：编辑态 apiKey 留空 = 用库里的密钥 */
export interface FetchModelsInput {
  serviceProvider?: string
  baseUrl: string
  apiKey?: string
}

/** 检测连通：只读，不落库（02-后端设计.md §1.8） */
export function test(name: string): Promise<ProviderTestResult> {
  return request<ProviderTestResult>(
    `/api/v1/service-providers/${encodeURIComponent(name)}/test`,
    { method: 'POST' },
  )
}

/** 拉取上游模型标识；只把 models 数组交给调用方（不落库） */
export async function fetchModels(input: FetchModelsInput): Promise<string[]> {
  const data = await request<{ models: string[] }>('/api/v1/service-providers/models:fetch', {
    method: 'POST',
    body: JSON.stringify(input),
  })
  return data.models
}

/**
 * 激活服务商（ADR-0015 / 02-后端设计.md §1.9）：空请求体，返回被激活的那一条。
 * 需求里没有"取消激活"，所以是单向动作。
 */
export function activate(name: string): Promise<Provider> {
  return request<Provider>(`/api/v1/service-providers/${encodeURIComponent(name)}:activate`, {
    method: 'POST',
  })
}

export function remove(name: string): Promise<void> {
  return request<void>(`/api/v1/service-providers/${encodeURIComponent(name)}`, { method: 'DELETE' })
}
