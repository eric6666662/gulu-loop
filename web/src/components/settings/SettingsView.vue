<script setup lang="ts">
import { inject, ref } from 'vue'
import { Plus, X } from 'lucide-vue-next'
import type { Provider, ProviderInput } from '../../api/providers'
import type { UseProviders } from '../../composables/useProviders'
import type { UseSettingsPane } from '../../composables/useSettingsPane'
import { useToast } from '../../composables/useToast'
import ConfirmDialog from '../dialog/ConfirmDialog.vue'
import ProviderDetail from './ProviderDetail.vue'
import ProviderList from './ProviderList.vue'
import SettingsEmpty from './SettingsEmpty.vue'

const emit = defineEmits<{ back: [] }>()

const { providers, loading, create, update, remove } = inject('providers') as UseProviders
const { step, editing, editProvider, createProvider, backToList } = inject('settingsPane') as UseSettingsPane
const { push } = useToast()

const saving = ref(false)
const formError = ref('')
const confirmOpen = ref(false)
const pendingDelete = ref<Provider | null>(null)

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

function onEdit(p: Provider): void {
  formError.value = ''
  editProvider(p)
}

function onCreate(): void {
  formError.value = ''
  createProvider()
}

function onBack(): void {
  formError.value = ''
  backToList()
}

async function onSave(input: ProviderInput): Promise<void> {
  saving.value = true
  formError.value = ''
  try {
    if (editing.value) await update(editing.value.serviceProvider, input)
    else await create(input)
    push('服务商已保存')
    backToList()
  } catch (e) {
    // 后端 400/409 的 message 可直出（02-后端设计.md §1.6），展示在表单顶部
    formError.value = message(e)
  } finally {
    saving.value = false
  }
}

function askRemove(p: Provider): void {
  pendingDelete.value = p
  confirmOpen.value = true
}

async function doRemove(): Promise<void> {
  const target = pendingDelete.value
  confirmOpen.value = false
  if (!target) return
  try {
    await remove(target.serviceProvider)
    push('服务商已删除')
    if (step.value === 'edit') backToList()
  } catch (e) {
    push(message(e), 'danger')
  }
}

// 检测连通在任务 3 接上；先占位不动作
function onTest(_p: Provider): void {}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <header class="flex h-14 shrink-0 items-center gap-2 border-b border-border bg-surface px-4 sm:px-8 lg:px-12">
      <h2 class="min-w-0 shrink-0 truncate text-title font-medium text-ink">服务商设置</h2>
      <p class="hidden min-w-0 truncate text-meta text-ink-muted sm:block">共 {{ providers.length }} 个服务商</p>
      <div class="ml-auto flex shrink-0 items-center gap-1">
        <button
          type="button"
          class="flex h-8 items-center gap-2 rounded-md px-2 text-meta font-medium text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
          @click="emit('back')"
        >
          <X class="size-4" :stroke-width="1.5" />返回聊天
        </button>
      </div>
    </header>

    <!-- 列表页 -->
    <div v-if="step === 'list'" class="flex min-h-0 flex-1 flex-col">
      <div class="flex shrink-0 items-center gap-2 px-4 py-3 sm:px-8 lg:px-12">
        <p class="text-meta text-ink-muted">
          点行尾的 <span class="text-ink">✎</span> 编辑，<span class="text-ink">⋯</span> 里可以检测连通或删除。
        </p>
        <button
          type="button"
          data-create
          class="ml-auto flex h-9 shrink-0 items-center gap-2 rounded-md bg-primary px-4 text-body font-medium text-on-primary transition-colors duration-150 hover:bg-primary-hover"
          @click="onCreate"
        >
          <Plus class="size-4" :stroke-width="1.5" />添加服务商
        </button>
      </div>

      <div class="scroll-thin min-h-0 flex-1 overflow-y-auto">
        <ProviderList
          v-if="providers.length > 0"
          :providers="providers"
          @edit="onEdit"
          @test="onTest"
          @remove="askRemove"
        />
        <SettingsEmpty v-else-if="!loading" @create="onCreate" />
      </div>
    </div>

    <!-- 编辑页 -->
    <ProviderDetail
      v-else
      :provider="editing"
      :saving="saving"
      :error="formError"
      @save="onSave"
      @remove="editing && askRemove(editing)"
      @back="onBack"
    />

    <ConfirmDialog
      :open="confirmOpen"
      :title="`删除「${pendingDelete?.serviceProvider ?? ''}」？`"
      :message="`这个服务商下的 ${pendingDelete?.models.length ?? 0} 个模型会一起删掉，模型切换器里也不再有它们。已有会话里的历史消息不受影响。`"
      confirm-text="删除服务商"
      @confirm="doRemove"
      @cancel="confirmOpen = false"
    />
  </div>
</template>
