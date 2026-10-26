<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { CircleAlert, CloudDownload, RefreshCw, X } from 'lucide-vue-next'
import { fetchModels } from '../../api/providers'

// 拉取模型列表弹层（02-ui设计.md ModelFetchDialog）：只读，勾选结果交给调用方追加到表单。
const props = defineProps<{
  open: boolean
  providerName: string
  serviceProvider?: string
  baseUrl: string
  apiKey: string
  existingIds: string[]
}>()
const emit = defineEmits<{ close: []; confirm: [ids: string[]] }>()

const loading = ref(false)
const error = ref('')
const ids = ref<string[]>([])
const selected = ref<string[]>([])

const existing = computed(() => new Set(props.existingIds))
const selectedCount = computed(() => selected.value.length)

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  selected.value = []
  try {
    ids.value = await fetchModels({
      serviceProvider: props.serviceProvider,
      baseUrl: props.baseUrl,
      apiKey: props.apiKey,
    })
  } catch (e) {
    ids.value = []
    // 502 时展示后端 message（可直出），弹层不关，用户可改 key 重拉或直接手填
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) void load()
  },
  { immediate: true },
)

function toggle(id: string, checked: boolean): void {
  if (existing.value.has(id)) return
  selected.value = checked ? [...selected.value, id] : selected.value.filter((x) => x !== id)
}

function confirm(): void {
  // 只提交新增的 id，重复的由调用方再兜一次
  emit('confirm', selected.value.filter((id) => !existing.value.has(id)))
}
</script>

<template>
  <div
    v-if="open"
    role="dialog"
    aria-modal="true"
    aria-labelledby="fetch-title"
    class="fixed inset-0 z-50 flex items-center justify-center p-4"
  >
    <div class="absolute inset-0 bg-overlay" @click="emit('close')"></div>
    <div
      class="relative flex max-h-full w-full max-w-content flex-col rounded-lg border border-border bg-surface shadow-overlay"
    >
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border px-6 py-4">
        <div>
          <h2 id="fetch-title" class="text-title font-semibold text-ink">从 {{ providerName }} 拉取模型</h2>
          <p class="mt-1 text-meta text-ink-muted">
            用这个服务商的 base_url 和 api_key 调上游 /v1/models。只读，不改动已保存的清单。
          </p>
        </div>
        <button
          type="button"
          data-cancel
          class="-mr-1 flex size-8 shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
          aria-label="关闭"
          @click="emit('close')"
        >
          <X class="size-4" :stroke-width="1.5" />
        </button>
      </div>

      <div class="scroll-thin min-h-0 flex-1 overflow-y-auto px-6 py-4">
        <div
          v-if="error"
          data-fetch-error
          class="flex items-start gap-2 rounded-md border border-border bg-danger-soft p-3"
        >
          <CircleAlert class="mt-1 size-4 shrink-0 text-danger" :stroke-width="1.5" />
          <div>
            <p class="text-body font-medium text-danger">{{ error }}</p>
            <p class="mt-1 text-meta text-ink">
              改好配置再点「重新拉取」，或者关掉这个窗口直接手填模型标识。已填的连接信息和模型行都还在。
            </p>
          </div>
        </div>

        <div class="mt-3 flex items-center gap-2">
          <p class="text-meta text-ink-muted">
            上游返回 <span data-numeric>{{ ids.length }}</span> 个模型，其中
            {{ existingIds.filter((id) => ids.includes(id)).length }} 个已在清单里
          </p>
          <button
            type="button"
            class="ml-auto flex h-8 items-center gap-2 rounded-md px-2 text-meta font-medium text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
            @click="load"
          >
            <RefreshCw class="size-4" :stroke-width="1.5" />重新拉取
          </button>
        </div>

        <p v-if="loading" class="mt-2 text-meta text-ink-muted">正在从上游拉取…</p>
        <p v-else-if="!error && ids.length === 0" class="mt-2 text-meta text-ink-muted">
          上游没有返回任何模型，检查 base_url 是否是 OpenAI 兼容接口，或者直接手填模型标识。
        </p>

        <ul class="mt-2 flex flex-col gap-1">
          <li v-for="id in ids" :key="id">
            <label
              class="flex items-center gap-3 rounded-md px-2 py-2 transition-colors duration-150"
              :class="existing.has(id) ? 'text-ink-subtle' : 'hover:bg-surface-alt'"
            >
              <input
                type="checkbox"
                data-fetch-item
                class="size-4 accent-primary"
                :disabled="existing.has(id)"
                @change="toggle(id, ($event.target as HTMLInputElement).checked)"
              />
              <span class="min-w-0 flex-1 font-mono text-meta" :class="existing.has(id) ? '' : 'text-ink'">{{
                id
              }}</span>
              <span v-if="existing.has(id)" data-added class="shrink-0 text-caption">已添加</span>
            </label>
          </li>
        </ul>
      </div>

      <div class="flex shrink-0 items-center gap-2 border-t border-border px-6 py-4">
        <p class="text-caption text-ink-muted">勾选的会追加到模型清单，重复的自动跳过。</p>
        <div class="ml-auto flex items-center gap-2">
          <button
            type="button"
            data-cancel
            class="h-9 rounded-md border border-border-strong px-4 text-body font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
            @click="emit('close')"
          >
            取消
          </button>
          <button
            type="button"
            data-confirm
            :disabled="selectedCount === 0"
            class="flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-body font-medium text-on-primary transition-colors duration-150 hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-surface-alt disabled:text-ink-subtle"
            @click="confirm"
          >
            <CloudDownload class="size-4" :stroke-width="1.5" />添加选中的 {{ selectedCount }} 个
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
