<script setup lang="ts">
import { Menu, Pencil, RefreshCw, Share } from 'lucide-vue-next'

// 会话头（02-ui设计.md ChatHeader）：标题 + 刷新 / 编辑标题 / 分享（+ 窄屏 ☰）
defineProps<{ title: string }>()
const emit = defineEmits<{ refresh: []; rename: []; export: []; 'open-sidebar': [] }>()
</script>

<template>
  <header class="flex h-14 shrink-0 items-center gap-2 border-b border-border bg-surface px-4 sm:px-8 lg:px-12">
    <h1 class="min-w-0 truncate text-title font-medium text-ink">{{ title || '新的会话' }}</h1>
    <div class="ml-auto flex shrink-0 items-center gap-1">
      <button
        type="button"
        data-refresh
        class="flex size-8 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
        aria-label="刷新当前会话"
        @click="emit('refresh')"
      >
        <RefreshCw class="size-4" :stroke-width="1.5" />
      </button>
      <button
        type="button"
        data-rename
        class="flex size-8 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
        aria-label="编辑会话标题"
        @click="emit('rename')"
      >
        <Pencil class="size-4" :stroke-width="1.5" />
      </button>
      <button
        type="button"
        data-export
        class="flex size-8 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
        aria-label="分享会话，下载 Markdown"
        @click="emit('export')"
      >
        <Share class="size-4" :stroke-width="1.5" />
      </button>
      <button
        type="button"
        data-sidebar-open
        class="flex size-8 shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink lg:hidden"
        aria-label="打开会话栏"
        @click="emit('open-sidebar')"
      >
        <Menu class="size-4" :stroke-width="1.5" />
      </button>
    </div>
  </header>
</template>
