<script setup lang="ts">
import { ref, watch } from 'vue'

// 编辑用户消息并重发（02-ui设计.md EditMessageDialog）
const props = defineProps<{ open: boolean; initialContent: string }>()
const emit = defineEmits<{ save: [content: string]; cancel: [] }>()

const content = ref('')
const error = ref('')

watch(
  () => props.open,
  (open) => {
    if (open) {
      content.value = props.initialContent
      error.value = ''
    }
  },
  { immediate: true },
)

function onSubmit(): void {
  const trimmed = content.value.trim()
  if (!trimmed) {
    error.value = '消息不能为空，改完再保存。'
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
    aria-labelledby="edit-msg-title"
    class="fixed inset-0 z-50 flex items-center justify-center p-4"
  >
    <div class="absolute inset-0 bg-overlay" @click="emit('cancel')"></div>
    <div class="relative w-full max-w-form rounded-lg border border-border bg-surface p-6 shadow-overlay">
      <h2 id="edit-msg-title" class="text-title font-semibold text-ink">改一下这条消息</h2>
      <p class="mt-1 text-meta text-ink-muted">保存后会删掉它之后的回复，并用新内容重新提问。</p>
      <textarea
        data-content-input
        v-model="content"
        rows="4"
        :aria-invalid="error ? 'true' : 'false'"
        class="mt-4 block w-full resize-none rounded-md border bg-surface px-3 py-2 text-body text-ink transition-colors duration-150 hover:border-ink-subtle focus:border-primary"
        :class="error ? 'border-danger' : 'border-border-strong'"
      ></textarea>
      <p class="mt-2 text-caption" :class="error ? 'text-danger' : 'text-ink-muted'">
        {{ error || '后面的回复会被删掉，无法恢复。' }}
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
          保存并重发
        </button>
      </div>
    </div>
  </div>
</template>
