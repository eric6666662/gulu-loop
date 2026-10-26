<script setup lang="ts">
import { Plus, Settings } from 'lucide-vue-next'

// 任务 1：只有品牌 + 底部两个入口；会话列表与窄屏抽屉由任务 4 接入
defineEmits<{ 'open-settings': []; 'new-chat': [] }>()
</script>

<template>
  <aside
    class="flex w-56 shrink-0 flex-col border-r border-border bg-surface-alt"
    aria-label="会话栏"
  >
    <div class="flex h-14 shrink-0 items-center pr-2 pl-3">
      <span class="text-body font-semibold tracking-tight text-ink">咕噜</span>
    </div>

    <div class="min-h-0 flex-1 overflow-y-auto"></div>

    <div class="flex shrink-0 items-center gap-2 border-t border-border p-2">
      <button
        type="button"
        data-open-settings
        class="flex size-8 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-surface hover:text-ink"
        aria-label="服务商设置"
        @click="$emit('open-settings')"
      >
        <Settings class="size-4" :stroke-width="1.5" />
      </button>
      <button
        type="button"
        class="ml-auto flex h-8 items-center gap-2 rounded-md bg-primary px-3 text-meta font-medium text-on-primary transition-colors duration-150 hover:bg-primary-hover"
        @click="$emit('new-chat')"
      >
        <Plus class="size-4" :stroke-width="1.5" />新的聊天
      </button>
    </div>
  </aside>
</template>
