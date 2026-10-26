<script setup lang="ts">
import { CircleAlert } from 'lucide-vue-next'

// 无服务商横幅（02-ui设计.md ProviderBanner）：说清"缺什么 + 怎么补"
defineEmits<{ 'open-settings': [] }>()
</script>

<template>
  <div class="shrink-0 border-b border-border bg-warning-soft px-4 py-3 sm:px-8 lg:px-12">
    <div class="flex items-center gap-3">
      <CircleAlert class="size-4 shrink-0 text-warning" :stroke-width="1.5" />
      <p class="text-meta text-ink">还没有可用的服务商。填好 base_url、api_key 和模型之后才能发送消息。</p>
      <button
        type="button"
        data-open-settings
        class="ml-auto h-8 shrink-0 rounded-md border border-border-strong bg-surface px-3 text-meta font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
        @click="$emit('open-settings')"
      >
        添加服务商
      </button>
    </div>
  </div>
</template>
