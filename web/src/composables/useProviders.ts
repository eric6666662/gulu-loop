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
  /** 激活的服务商（全局恰好 0 或 1 个）；无激活时为 null（ADR-0015） */
  activeProvider: ComputedRef<Provider | null>
  models: ComputedRef<ProviderModel[]>
  loading: Ref<boolean>
  error: Ref<string>
  load: () => Promise<void>
  test: (name: string) => Promise<api.ProviderTestResult>
  activate: (name: string) => Promise<Provider>
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

  // 激活的服务商：全局唯一，来自后端的 isActive（ADR-0015）
  const activeProvider = computed<Provider | null>(
    () => providers.value.find((p) => p.isActive) ?? null,
  )

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

  /**
   * 激活服务商：成功后把返回的那一条**整条替换**进本地列表、其余 isActive 置 false，
   * 不重拉列表（02-后端设计.md §5.2 G）。失败原样抛给页面弹 toast。
   */
  async function activate(name: string): Promise<Provider> {
    const updated = await api.activate(name)
    providers.value = providers.value.map((p) =>
      p.serviceProvider === updated.serviceProvider ? updated : { ...p, isActive: false },
    )
    return updated
  }

  async function remove(name: string): Promise<void> {
    await api.remove(name)
    providers.value = providers.value.filter((p) => p.serviceProvider !== name)
  }

  /** 新增：成功后把新服务商并进本地列表（models 自动重算），失败原样抛给页面展示 */
  /** 检测连通：只调接口并返回结果，弹什么提示由页面决定（失败原因在 ApiError.message 里） */
  async function test(name: string): Promise<api.ProviderTestResult> {
    return api.test(name)
  }

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

  return { providers, activeProvider, models, loading, error, load, test, activate, create, update, remove }
}
