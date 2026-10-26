<script setup lang="ts">
import { ref } from 'vue'
import { ArrowUp, Square } from 'lucide-vue-next'

const props = defineProps<{ streaming: boolean; disabled?: boolean }>()
const emit = defineEmits<{ send: [text: string]; stop: [] }>()

const text = ref('')
const textarea = ref<HTMLTextAreaElement | null>(null)

function autoGrow(): void {
  const el = textarea.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 160)}px`
}

function submit(): void {
  const value = text.value.trim()
  if (!value) return
  emit('send', value)
  text.value = ''
  autoGrow()
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key !== 'Enter' || e.shiftKey || e.isComposing) return
  e.preventDefault()
  submit()
}

function onPrimary(): void {
  if (props.streaming) emit('stop')
  else submit()
}

defineExpose({
  focus: () => textarea.value?.focus(),
  clear: () => {
    text.value = ''
    autoGrow()
  },
})
</script>

<template>
  <div class="px-4 pb-4 sm:px-8 lg:px-12">
    <div
      class="flex flex-col rounded-xl border transition-colors duration-150 focus-within:border-primary focus-within:ring-2 focus-within:ring-primary/20"
      :class="disabled ? 'border-border bg-surface-alt' : 'border-border-strong bg-surface'"
      data-composer
    >
      <textarea
        ref="textarea"
        v-model="text"
        data-composer-textarea
        rows="1"
        :disabled="disabled"
        :placeholder="disabled ? '当前不可发送' : '输入消息…'"
        class="max-h-40 resize-none bg-transparent px-3 pt-3 text-body-lg text-ink outline-none placeholder:text-ink-subtle"
        :class="disabled ? 'text-ink-subtle' : ''"
        @input="autoGrow"
        @keydown="onKeydown"
      ></textarea>

      <div class="flex h-10 shrink-0 items-center gap-1 px-3">
        <button
          type="button"
          data-primary
          :disabled="disabled || (!streaming && text.trim() === '')"
          :aria-label="streaming ? '停止生成' : '发送'"
          class="ml-auto flex size-8 items-center justify-center rounded-md bg-primary text-on-primary transition-colors duration-150 hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-surface-alt disabled:text-ink-subtle"
          @click="onPrimary"
        >
          <Square v-if="streaming" class="size-4" :stroke-width="1.5" />
          <ArrowUp v-else class="size-4" :stroke-width="1.5" />
        </button>
      </div>
    </div>
  </div>
</template>
