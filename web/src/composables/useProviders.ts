import { computed, ref, type ComputedRef, type Ref } from 'vue'
import * as api from '../api/providers'
import type { Provider } from '../api/providers'

/** 模型切换器用的扁平模型项：值取 id、标签取 name、副标题用服务商标识 */
export interface ProviderModel {
  id: string
  name: string
  serviceProvider: string
}

export interface UseProviders {
  providers: Ref<Provider[]>
  models: ComputedRef<ProviderModel[]>
  loading: Ref<boolean>
  error: Ref<string>
  load: () => Promise<void>
  create: (input: api.ProviderInput) => Promise<Provider>
  update: (name: string, input: api.ProviderInput) => Promise<Provider>
  remove: (name: string) => Promise<void>
}

/**
 * 服务商数据的唯一入口（02-前端设计.md §2.1）。
 *
 * `models` 不单独存，由 `providers` 派生——服务商一变，模型列表自动重算，
 * 不会出现"删了服务商但模型下拉里还有"的状态。
 */
export function useProviders(): UseProviders {
  const providers = ref<Provider[]>([])
  const loading = ref(false)
  const error = ref('')

  const models = computed<ProviderModel[]>(() =>
    providers.value.flatMap((p) =>
      p.models.map((m) => ({
        id: m.id,
        name: m.name || m.id,
        serviceProvider: p.serviceProvider,
      })),
    ),
  )

  async function load(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      providers.value = await api.list()
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  async function remove(name: string): Promise<void> {
    await api.remove(name)
    providers.value = providers.value.filter((p) => p.serviceProvider !== name)
  }

  /** 新增：成功后把新服务商并进本地列表（models 自动重算），失败原样抛给页面展示 */
  async function create(input: api.ProviderInput): Promise<Provider> {
    const created = await api.create(input)
    providers.value = [...providers.value, created].sort((a, b) =>
      a.serviceProvider.localeCompare(b.serviceProvider),
    )
    return created
  }

  /** 更新：成功后整条替换（同名），models 随之重算 */
  async function update(name: string, input: api.ProviderInput): Promise<Provider> {
    const updated = await api.update(name, input)
    providers.value = providers.value
      .map((p) => (p.serviceProvider === name ? updated : p))
      .sort((a, b) => a.serviceProvider.localeCompare(b.serviceProvider))
    return updated
  }

  return { providers, models, loading, error, load, create, update, remove }
}
