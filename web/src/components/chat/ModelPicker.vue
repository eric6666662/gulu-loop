<script setup lang="ts">
import { Check } from 'lucide-vue-next'
import type { ProviderModel } from '../../composables/useProviders'

// 切换模型弹层（02-ui设计.md ModelPicker）：从按钮向上弹出、与按钮左缘对齐、w-64
const props = defineProps<{ models: ProviderModel[]; modelValue: string; provider?: string }>()
const emit = defineEmits<{ 'update:modelValue': [model: string]; close: [] }>()

function choose(id: string): void {
  emit('update:modelValue', id)
  emit('close')
}
</script>

<template>
  <div
    data-model-picker
    role="listbox"
    aria-label="选择模型"
    class="absolute bottom-full left-0 z-50 mb-2 w-64 rounded-lg border border-border bg-surface p-1 shadow-overlay"
  >
    <p class="px-2 py-1 text-caption text-ink-muted">
      <span class="text-ink">{{ provider }}</span> 的模型 · 这个会话的后续消息都用它
    </p>

    <button
      v-for="m in props.models"
      :key="m.serviceProvider + '/' + m.id"
      type="button"
      role="option"
      :data-model-option="m.id"
      :aria-selected="m.id === props.modelValue ? 'true' : 'false'"
      class="flex w-full items-center gap-2 rounded-sm px-2 py-2 text-left transition-colors duration-150 hover:bg-surface-alt"
      @click="choose(m.id)"
    >
      <span
        class="min-w-0 flex-1 truncate text-body text-ink"
        :class="m.id === props.modelValue ? 'font-medium' : ''"
        >{{ m.name || m.id }}</span
      >
      <Check v-if="m.id === props.modelValue" class="size-4 shrink-0 text-primary" :stroke-width="1.5" />
    </button>
  </div>
</template>
