<script setup lang="ts">
import { Activity, Trash2 } from 'lucide-vue-next'

// 行内下拉抽屉（02-ui设计.md ProviderRowMenu）：检测连通 / 删除
defineEmits<{ test: []; remove: [] }>()
</script>

<template>
  <div
    role="menu"
    class="absolute top-full right-0 z-20 mt-1 w-40 rounded-lg border border-border bg-surface p-1 shadow-overlay"
  >
    <button
      type="button"
      role="menuitem"
      data-menu-test
      class="flex h-8 w-full items-center gap-2 rounded-sm px-2 text-meta text-ink transition-colors duration-150 hover:bg-surface-alt"
      @click="$emit('test')"
    >
      <Activity class="size-4 shrink-0" :stroke-width="1.5" />检测连通
    </button>
    <button
      type="button"
      role="menuitem"
      data-menu-remove
      class="flex h-8 w-full items-center gap-2 rounded-sm px-2 text-meta text-danger transition-colors duration-150 hover:bg-danger-soft"
      @click="$emit('remove')"
    >
      <Trash2 class="size-4 shrink-0" :stroke-width="1.5" />删除
    </button>
  </div>
</template>
