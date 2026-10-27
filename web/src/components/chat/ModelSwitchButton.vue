<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Bot } from 'lucide-vue-next'
import type { ProviderModel } from '../../composables/useProviders'
import ModelPicker from './ModelPicker.vue'

// 切换模型按钮（02-ui设计.md ModelSwitchButton）：robot 图标 + 当前模型名，弹层从按钮向上弹出
const props = defineProps<{
  models: ProviderModel[]
  modelValue: string
  provider?: string
  disabled?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [model: string] }>()

const open = ref(false)

function toggle(): void {
  if (props.disabled || props.models.length === 0) return
  open.value = !open.value
}
function close(): void {
  open.value = false
}
function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') close()
}
function onDocClick(e: MouseEvent): void {
  const el = e.target as HTMLElement | null
  if (!el?.closest('[data-model-root]')) close()
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  document.addEventListener('click', onDocClick)
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  document.removeEventListener('click', onDocClick)
})
</script>

<template>
  <div class="relative" data-model-root>
    <button
      type="button"
      data-model-switch
      :disabled="disabled || models.length === 0"
      aria-haspopup="listbox"
      :aria-expanded="open ? 'true' : 'false'"
      :aria-label="`切换模型，当前 ${modelValue || '未选择'}`"
      class="peer flex h-8 items-center gap-2 rounded-md px-2 text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink disabled:cursor-not-allowed disabled:text-ink-subtle"
      @click.stop="toggle"
    >
      <Bot class="size-4 shrink-0" :stroke-width="1.5" />
      <span class="max-w-40 truncate text-meta font-medium" data-model-name>{{ modelValue || '选择模型' }}</span>
    </button>

    <span
      class="pointer-events-none invisible absolute bottom-full left-0 mb-2 rounded-sm bg-ink px-2 py-1 text-caption whitespace-nowrap text-on-primary opacity-0 shadow-raised transition-opacity duration-150 peer-focus-visible:visible peer-focus-visible:opacity-100 peer-hover:visible peer-hover:opacity-100"
      role="tooltip"
      >切换模型</span
    >

    <ModelPicker
      v-if="open"
      :models="models"
      :model-value="modelValue"
      :provider="provider"
      @update:model-value="emit('update:modelValue', $event)"
      @close="close"
    />
  </div>
</template>
