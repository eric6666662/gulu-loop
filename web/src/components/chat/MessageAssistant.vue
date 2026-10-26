<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import type { Message } from '../../api/messages'
import { renderMarkdown } from '../../lib/markdown'
import { relativeTime } from '../../lib/format'

// 助手消息：无气泡、无边框，只有正文 + 落款行（02-ui设计.md MessageAssistant）
const props = defineProps<{ message: Message }>()

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
      <time class="ml-auto text-caption text-ink-muted">{{ relativeTime(message.createdAt) }}</time>
    </div>
  </div>
</template>
