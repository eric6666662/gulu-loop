<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { Copy, RefreshCw } from 'lucide-vue-next'
import type { Message } from '../../api/messages'
import { renderMarkdown } from '../../lib/markdown'
import { relativeTime } from '../../lib/format'

// 助手消息：无气泡、无边框，只有正文 + 落款行（02-ui设计.md MessageAssistant）
const props = defineProps<{ message: Message }>()
const emit = defineEmits<{ retry: []; copy: [] }>()

const html = ref('')

async function render(): Promise<void> {
  html.value = await renderMarkdown(props.message.content)
}

onMounted(render)
watch(() => props.message.content, render)

/** 代码块的复制按钮（内容由 markdown 渲染，用事件委托统一处理） */
async function onCopyCode(e: MouseEvent): Promise<void> {
  const btn = (e.target as HTMLElement).closest('[data-copy-code]')
  if (!btn) return
  const code = btn.parentElement?.querySelector('code')?.textContent ?? ''
  try {
    await navigator.clipboard.writeText(code)
  } catch {
    /* 剪贴板不可用时静默（本地 http 环境偶尔会拒绝） */
  }
}
</script>

<template>
  <div data-message class="group mt-6">
    <!-- eslint-disable-next-line vue/no-v-html -- 内容来自 markdown-it（html:false，已转义） -->
    <div data-body class="msg-body text-body-lg text-ink" @click="onCopyCode" v-html="html"></div>

    <div data-footer class="mt-3 flex items-center gap-2">
      <span class="text-caption text-ink-muted">{{ message.model }}</span>
      <!-- 操作组：hover / focus-within 才出现（复制在任务 9） -->
      <div
        class="ml-auto flex items-center gap-1 opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100"
      >
        <button
          type="button"
          data-retry
          aria-label="重试这条回复"
          class="flex size-7 items-center justify-center rounded-sm text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
          @click="emit('retry')"
        >
          <RefreshCw class="size-4" :stroke-width="1.5" />
        </button>
        <button
          type="button"
          data-copy
          aria-label="复制这条回复"
          class="flex size-7 items-center justify-center rounded-sm text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
          @click="emit('copy')"
        >
          <Copy class="size-4" :stroke-width="1.5" />
        </button>
      </div>
      <time class="text-caption text-ink-muted">{{ relativeTime(message.createdAt) }}</time>
    </div>
  </div>
</template>
