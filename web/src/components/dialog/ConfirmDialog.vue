<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'

// 通用二次确认弹层（02-ui设计.md：删除会话 / 删除服务商共用一个组件，只是文案不同）
defineProps<{
  open: boolean
  title: string
  message: string
  confirmText: string
}>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') emit('cancel')
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div
    v-if="open"
    role="dialog"
    aria-modal="true"
    aria-labelledby="confirm-title"
    class="fixed inset-0 z-50 flex items-center justify-center p-4"
  >
    <div class="absolute inset-0 bg-overlay" @click="emit('cancel')"></div>
    <div
      class="relative w-full max-w-form rounded-lg border border-border bg-surface p-6 shadow-overlay"
    >
      <h2 id="confirm-title" class="text-title font-semibold text-ink">{{ title }}</h2>
      <p class="mt-2 text-meta text-ink-muted">{{ message }}</p>
      <div class="mt-6 flex justify-end gap-2">
        <button
          type="button"
          data-cancel
          class="h-9 rounded-md border border-border-strong px-4 text-body font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
          @click="emit('cancel')"
        >
          先留着
        </button>
        <button
          type="button"
          data-confirm
          class="h-9 rounded-md bg-danger px-4 text-body font-medium text-on-primary transition-opacity duration-150 hover:opacity-90"
          @click="emit('confirm')"
        >
          {{ confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>
