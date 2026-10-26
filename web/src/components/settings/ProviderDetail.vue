<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { Check, ChevronLeft, CircleAlert, CloudDownload, Plus, Trash2 } from 'lucide-vue-next'
import type { ModelRef, Provider, ProviderInput } from '../../api/providers'
import ModelRow from './ModelRow.vue'

const props = defineProps<{ provider: Provider | null; saving?: boolean; error?: string }>()
const emit = defineEmits<{
  save: [input: ProviderInput]
  remove: []
  back: []
  'fetch-models': []
}>()

const isNew = computed(() => props.provider === null)

interface Form {
  serviceProvider: string
  website: string
  baseUrl: string
  apiKey: string
  model: string
  models: ModelRef[]
}

const form = reactive<Form>({ serviceProvider: '', website: '', baseUrl: '', apiKey: '', model: '', models: [] })
const errors = reactive<Record<'name' | 'baseUrl' | 'apiKey' | 'models', string>>({
  name: '',
  baseUrl: '',
  apiKey: '',
  models: '',
})
const submitted = ref(false)

/** 换服务商 / 切新增态时整体重置表单；api_key 永远从空开始（留空 = 不修改） */
watch(
  () => props.provider,
  (p) => {
    form.serviceProvider = p?.serviceProvider ?? ''
    form.website = p?.website ?? ''
    form.baseUrl = p?.baseUrl ?? ''
    form.apiKey = ''
    form.model = p?.model ?? ''
    form.models = p ? p.models.map((m) => ({ ...m })) : []
    submitted.value = false
    clearErrors()
  },
  { immediate: true },
)

function clearErrors(): void {
  errors.name = ''
  errors.baseUrl = ''
  errors.apiKey = ''
  errors.models = ''
}

function validate(): boolean {
  clearErrors()
  submitted.value = true

  if (!form.serviceProvider.trim()) errors.name = '服务商名称不能为空。用一个短标识，例如 deepseek。'
  if (!form.baseUrl.trim()) {
    errors.baseUrl = 'Base URL 不能为空。填 OpenAI 兼容地址，例如 https://api.deepseek.com/v1。'
  }
  if (isNew.value && !form.apiKey.trim()) errors.apiKey = 'api_key 不能为空。到服务商控制台复制一个 key。'
  if (form.models.length === 0) {
    errors.models = '至少配置一个模型。点「拉取模型列表」挑，或者手动填模型标识。'
  } else if (form.models.some((m) => !m.id.trim())) {
    errors.models = '模型标识不能为空。'
  } else {
    const ids = form.models.map((m) => m.id.trim().toLowerCase())
    const dup = ids.find((id, i) => ids.indexOf(id) !== i)
    if (dup) errors.models = `模型标识重复：${form.models[ids.indexOf(dup)].id}`
  }

  return !errors.name && !errors.baseUrl && !errors.apiKey && !errors.models
}

function onSubmit(): void {
  if (!validate()) return
  emit('save', {
    serviceProvider: form.serviceProvider.trim(),
    website: form.website.trim(),
    baseUrl: form.baseUrl.trim(),
    apiKey: form.apiKey.trim(), // 空串 = 后端不修改已存密钥
    model: form.model || form.models[0]?.id || '',
    models: form.models.map((m) => ({ id: m.id.trim(), name: m.name.trim() })),
  })
}

function updateModel(index: number, next: ModelRef): void {
  form.models[index] = next
  // 标识改了要同步默认模型，否则默认会指向一个已不存在的 id
  if (form.model === form.models[index].id) form.model = next.id
}

function removeModel(index: number): void {
  const [removed] = form.models.splice(index, 1)
  if (removed && form.model === removed.id) form.model = form.models[0]?.id ?? ''
}

function setDefault(index: number): void {
  form.model = form.models[index]?.id ?? ''
}

function addModel(): void {
  const id = `model-${form.models.length + 1}`
  form.models.push({ id: '', name: '' })
  if (!form.model) form.model = id === '' ? '' : form.models[0]?.id ?? ''
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <!-- 返回行 -->
    <div class="flex shrink-0 items-center gap-2 px-4 py-3 sm:px-8 lg:px-12">
      <button
        type="button"
        data-back
        class="flex h-8 items-center gap-1 rounded-md px-2 text-meta font-medium text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
        @click="emit('back')"
      >
        <ChevronLeft class="size-4" :stroke-width="1.5" />服务商列表
      </button>
      <h3 class="ml-1 min-w-0 truncate text-title font-semibold text-ink" data-detail-title>
        {{ isNew ? '添加服务商' : provider?.serviceProvider }}
      </h3>
      <span
        v-if="!isNew"
        class="shrink-0 rounded-xs bg-surface-alt px-2 py-1 text-caption text-ink-muted"
        >标识 {{ provider?.serviceProvider }}</span
      >
      <button
        v-if="!isNew"
        type="button"
        data-remove
        class="ml-auto flex h-8 shrink-0 items-center gap-2 rounded-md px-2 text-meta font-medium text-ink-muted transition-colors duration-150 hover:bg-danger-soft hover:text-danger"
        @click="emit('remove')"
      >
        <Trash2 class="size-4" :stroke-width="1.5" />删除
      </button>
    </div>

    <div class="scroll-thin min-h-0 flex-1 overflow-y-auto">
      <div class="mx-auto flex w-full max-w-wide flex-col px-4 pb-6 sm:px-8 lg:px-12">
        <!-- 后端返回的错误（重名等） -->
        <div
          v-if="error"
          class="mb-3 flex items-start gap-2 rounded-md border border-border bg-danger-soft p-3"
          data-form-error
        >
          <CircleAlert class="mt-1 size-4 shrink-0 text-danger" :stroke-width="1.5" />
          <p class="text-meta text-danger">{{ error }}</p>
        </div>

        <!-- 基本信息 -->
        <section class="border-t border-border pt-4">
          <h4 class="text-body font-semibold text-ink">基本信息</h4>
          <div class="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label for="pd-name" class="block text-meta font-medium text-ink">名称</label>
              <input
                id="pd-name"
                data-name
                type="text"
                v-model="form.serviceProvider"
                :readonly="!isNew"
                :aria-invalid="errors.name ? 'true' : 'false'"
                placeholder="deepseek"
                class="mt-2 block h-9 w-full rounded-md border px-3 text-body text-ink transition-colors duration-150 focus:border-primary"
                :class="[
                  !isNew ? 'border-border bg-surface-alt text-ink-muted' : 'border-border-strong bg-surface hover:border-ink-subtle',
                  errors.name ? 'border-danger' : '',
                ]"
              />
              <p class="mt-2 text-caption" :class="errors.name ? 'text-danger' : 'text-ink-muted'">
                {{ errors.name || '唯一标识，创建后不能改；改名等于删了重建。' }}
              </p>
            </div>

            <div>
              <label for="pd-site" class="block text-meta font-medium text-ink">官网链接</label>
              <input
                id="pd-site"
                data-website
                type="url"
                v-model="form.website"
                placeholder="https://platform.deepseek.com"
                class="mt-2 block h-9 w-full rounded-md border border-border-strong bg-surface px-3 text-body text-ink transition-colors duration-150 hover:border-ink-subtle focus:border-primary"
              />
              <p class="mt-2 text-caption text-ink-muted">只用于列表里跳转，不参与调用。</p>
            </div>

            <div>
              <label for="pd-url" class="block text-meta font-medium text-ink">Base URL</label>
              <input
                id="pd-url"
                data-base-url
                type="url"
                v-model="form.baseUrl"
                :aria-invalid="errors.baseUrl ? 'true' : 'false'"
                aria-describedby="pd-url-hint"
                placeholder="https://api.deepseek.com/v1"
                class="mt-2 block h-9 w-full rounded-md border bg-surface px-3 text-body text-ink transition-colors duration-150 hover:border-ink-subtle focus:border-primary"
                :class="errors.baseUrl ? 'border-danger' : 'border-border-strong'"
              />
              <p id="pd-url-hint" class="mt-2 text-caption" :class="errors.baseUrl ? 'text-danger' : 'text-ink-muted'">
                {{ errors.baseUrl || 'OpenAI 兼容接口地址，通常以 /v1 结尾。' }}
              </p>
            </div>

            <div>
              <label for="pd-key" class="block text-meta font-medium text-ink">api_key</label>
              <div class="mt-2 flex items-center gap-2">
                <input
                  id="pd-key"
                  data-api-key
                  type="password"
                  v-model="form.apiKey"
                  :aria-invalid="errors.apiKey ? 'true' : 'false'"
                  :placeholder="isNew ? 'sk-…' : '已配置，留空则不修改'"
                  class="block h-9 min-w-0 flex-1 rounded-md border bg-surface px-3 font-mono text-meta text-ink transition-colors duration-150 hover:border-ink-subtle focus:border-primary"
                  :class="errors.apiKey ? 'border-danger' : 'border-border-strong'"
                />
                <span
                  v-if="!isNew && provider?.hasApiKey"
                  class="flex h-9 shrink-0 items-center gap-2 rounded-md border border-border bg-surface-alt px-3 text-caption text-ink-muted"
                >
                  <Check class="size-4 text-success" :stroke-width="1.5" />已配置
                </span>
              </div>
              <p class="mt-2 text-caption" :class="errors.apiKey ? 'text-danger' : 'text-ink-muted'">
                {{ errors.apiKey || '只存在本地 SQLite 里，不回显、不写日志。' }}
              </p>
            </div>
          </div>
        </section>

        <!-- 模型列表 -->
        <section class="mt-6 border-t border-border pt-4">
          <div class="flex flex-wrap items-center gap-2">
            <h4 class="text-body font-semibold text-ink">模型列表</h4>
            <span class="text-meta text-ink-muted"
              >{{ form.models.length }} 个 · 默认
              <span class="text-ink">{{ form.model || '—' }}</span></span
            >
            <button
              type="button"
              data-fetch-models
              class="ml-auto flex h-8 items-center gap-2 rounded-md border border-border-strong px-3 text-meta font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
              @click="emit('fetch-models')"
            >
              <CloudDownload class="size-4" :stroke-width="1.5" />拉取模型列表
            </button>
          </div>
          <p class="mt-1 text-caption text-ink-muted">
            手填模型标识，或者点「拉取模型列表」从上游 /v1/models 里挑。拉取只读，不改这里的清单。
          </p>

          <div class="mt-3 flex flex-col gap-2">
            <ModelRow
              v-for="(m, i) in form.models"
              :key="i"
              :model="m"
              :is-default="form.model === m.id"
              @update="updateModel(i, $event)"
              @remove="removeModel(i)"
              @set-default="setDefault(i)"
            />
          </div>

          <p v-if="errors.models" class="mt-2 text-caption text-danger">{{ errors.models }}</p>

          <button
            type="button"
            data-model-add
            class="mt-2 flex h-9 items-center gap-2 rounded-md border border-border-strong px-3 text-meta font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
            @click="addModel"
          >
            <Plus class="size-4" :stroke-width="1.5" />添加模型
          </button>
        </section>
      </div>
    </div>

    <!-- 保存栏 -->
    <div class="shrink-0 border-t border-border bg-surface px-4 py-3 sm:px-8 lg:px-12">
      <div class="mx-auto flex w-full min-w-0 max-w-wide items-center gap-2">
        <p class="hidden min-w-0 truncate text-caption text-ink-muted sm:block">
          api_key 只写不读；保存后模型切换器立即更新。
        </p>
        <div class="ml-auto flex items-center gap-2">
          <button
            type="button"
            data-cancel
            class="h-9 rounded-md border border-border-strong px-4 text-body font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
            @click="emit('back')"
          >
            取消
          </button>
          <button
            type="button"
            data-save
            :disabled="saving"
            class="h-9 rounded-md bg-primary px-4 text-body font-medium text-on-primary transition-colors duration-150 hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-surface-alt disabled:text-ink-subtle"
            @click="onSubmit"
          >
            保存服务商
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
