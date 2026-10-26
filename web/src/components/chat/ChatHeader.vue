<script setup lang="ts">
import { Menu, Pencil } from 'lucide-vue-next'

// 会话头（02-ui设计.md ChatHeader）。刷新 / 分享按钮分别在任务 5、9 接入，
// 现在不渲染，避免出现点了没反应的死按钮。
defineProps<{ title: string }>()
const emit = defineEmits<{ rename: []; 'open-sidebar': [] }>()
</script>

<template>
  <header class="flex h-14 shrink-0 items-center gap-2 border-b border-border bg-surface px-4 sm:px-8 lg:px-12">
    <h1 class="min-w-0 truncate text-title font-medium text-ink">{{ title || '新的会话' }}</h1>
    <div class="ml-auto flex shrink-0 items-center gap-1">
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
