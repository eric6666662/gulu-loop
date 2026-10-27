import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { Provider } from '../api/providers'
import type { ProviderModel } from './useProviders'

/**
 * 一次模型选择的完整身份：**哪个服务商的哪个模型**。
 *
 * 不能只用模型名当唯一键 —— 中转站与官方可能有同名模型（ADR-0015 / ADR-0016）。
 */
export interface SelectedModel {
  serviceProvider: string
  model: string
}

/** 模型回落：激活的服务商变了，当前模型不在新清单里，于是自动换成新服务商的默认模型。 */
export interface ModelFallback {
  from: string
  to: string
}

export interface UseModelSelection {
  /** 当前选中（null = 无激活服务商） */
  selected: Ref<SelectedModel | null>
  /** 候选模型：只来自激活的服务商 */
  models: ComputedRef<ProviderModel[]>
  /** 按钮与落款行用的短标签（就是模型名） */
  label: ComputedRef<string>
  /** 回落痕迹（供消息区展示）；留到下次发送为止 */
  fallback: Ref<ModelFallback | null>
  select: (m: SelectedModel) => void
  clearFallback: () => void
}

/**
 * 模型选择的唯一入口（02-前端设计.md §2.1 / ADR-0016）。
 *
 * 独吞四条规则：
 *   1. 初值 = 激活服务商的默认模型（`provider.model`）
 *   2. 激活的服务商一变，若当前模型仍在新清单里 → 只换 serviceProvider（不回落、不留痕迹）
 *   3. 不在新清单里 → 换成新服务商的默认模型，并记 `fallback`
 *   4. 无激活 → `selected` 为 null、候选为空
 *
 * 不碰服务商数据（只读传入的 activeProvider），也不碰消息与流（那是 useChat 的事）。
 */
export function useModelSelection(activeProvider: ComputedRef<Provider | null>): UseModelSelection {
  const selected = ref<SelectedModel | null>(null)
  const fallback = ref<ModelFallback | null>(null)

  const models = computed<ProviderModel[]>(() =>
    (activeProvider.value?.models ?? []).map((m) => ({
      id: m.id,
      name: m.name || m.id,
      serviceProvider: activeProvider.value?.serviceProvider ?? '',
    })),
  )

  const label = computed(() => selected.value?.model ?? '')

  /** 激活服务商的默认模型（`provider.model` 一定在清单内，由后端校验保证） */
  function defaultOf(p: Provider): SelectedModel {
    return { serviceProvider: p.serviceProvider, model: p.model }
  }

  watch(
    () => activeProvider.value?.serviceProvider,
    () => {
      const p = activeProvider.value
      if (!p) {
        selected.value = null
        return
      }
      const cur = selected.value
      // 没有选择 / 选的还是同一个服务商 → 直接对齐到它的默认模型
      if (!cur || cur.serviceProvider === p.serviceProvider) {
        if (!cur) selected.value = defaultOf(p)
        return
      }
      // 换了服务商：模型名还在新清单里就只换身份，否则回落
      const stillThere = p.models.some((m) => m.id === cur.model)
      if (stillThere) {
        selected.value = { serviceProvider: p.serviceProvider, model: cur.model }
        return
      }
      const next = defaultOf(p)
      selected.value = next
      fallback.value = { from: cur.model, to: next.model }
    },
    { immediate: true },
  )

  function select(m: SelectedModel): void {
    selected.value = m
  }

  function clearFallback(): void {
    fallback.value = null
  }

  return { selected, models, label, fallback, select, clearFallback }
}
