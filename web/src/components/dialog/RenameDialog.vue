<script setup lang="ts">
import { ref, watch } from 'vue'

// 重命名会话（02-ui设计.md RenameDialog）：预填当前标题，空标题前端先拦
const props = defineProps<{ open: boolean; initialTitle: string }>()
const emit = defineEmits<{ save: [title: string]; cancel: [] }>()

const title = ref('')
const error = ref('')

watch(
  () => props.open,
  (open) => {
    if (open) {
      title.value = props.initialTitle
      error.value = ''
    }
  },
  { immediate: true },
)

function onSubmit(): void {
  const trimmed = title.value.trim()
  if (!trimmed) {
    error.value = '标题不能为空。写个 20 字以内的说法。'
    return
  }
  emit('save', trimmed)
}
</script>

<template>
  <div
    v-if="open"
    role="dialog"
    aria-modal="true"
    aria-labelledby="rename-title"
    class="fixed inset-0 z-50 flex items-center justify-center p-4"
  >
    <div class="absolute inset-0 bg-overlay" @click="emit('cancel')"></div>
    <div class="relative w-full max-w-form rounded-lg border border-border bg-surface p-6 shadow-overlay">
      <h2 id="rename-title" class="text-title font-semibold text-ink">改个标题</h2>
      <label for="rename-input" class="mt-4 block text-meta font-medium text-ink">会话标题</label>
      <input
        id="rename-input"
        data-title-input
        v-model="title"
        type="text"
        :aria-invalid="error ? 'true' : 'false'"
        class="mt-2 block h-9 w-full rounded-md border bg-surface px-3 text-body text-ink transition-colors duration-150 hover:border-ink-subtle focus:border-primary"
        :class="error ? 'border-danger' : 'border-border-strong'"
        @keyup.enter="onSubmit"
      />
      <p class="mt-2 text-caption" :class="error ? 'text-danger' : 'text-ink-muted'">
        {{ error || '列表里显示的就是它，改完不影响已有消息。' }}
      </p>
      <div class="mt-6 flex justify-end gap-2">
        <button
          type="button"
          data-cancel
          class="h-9 rounded-md border border-border-strong px-4 text-body font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
          @click="emit('cancel')"
        >
          取消
        </button>
        <button
          type="button"
          data-save
          class="h-9 rounded-md bg-primary px-4 text-body font-medium text-on-primary transition-colors duration-150 hover:bg-primary-hover"
          @click="onSubmit"
        >
          保存标题
        </button>
      </div>
    </div>
  </div>
</template>
