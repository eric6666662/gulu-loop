<script setup lang="ts">
import { CircleAlert } from 'lucide-vue-next'

// 消息区的错误卡：拉历史失败 / 流式失败共用（02-ui设计.md StreamError）
defineProps<{ title: string; message: string }>()
defineEmits<{ retry: []; 'open-settings': [] }>()
</script>

<template>
  <div class="mt-6">
    <div class="rounded-lg border border-border bg-danger-soft p-4">
      <div class="flex items-start gap-3">
        <CircleAlert class="mt-1 size-4 shrink-0 text-danger" :stroke-width="1.5" />
        <div class="min-w-0">
          <p class="text-body font-medium text-danger">{{ title }}</p>
          <p class="mt-1 text-meta text-ink-muted">{{ message }}</p>
          <div class="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              data-retry
              class="h-8 rounded-md bg-danger px-3 text-meta font-medium text-on-primary transition-opacity duration-150 hover:opacity-90"
              @click="$emit('retry')"
            >
              重试
            </button>
            <button
              type="button"
              class="h-8 rounded-md border border-border-strong bg-surface px-3 text-meta font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
              @click="$emit('open-settings')"
            >
              打开服务商设置
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
