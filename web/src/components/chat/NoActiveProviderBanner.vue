<script setup lang="ts">
import { CircleAlert } from 'lucide-vue-next'

// 有服务商但没有激活（02-ui设计.md §3 NoActiveProviderBanner）：
// 与 ProviderBanner 互斥 —— 0 个服务商是「去添加」，有服务商没激活是「去激活」。
defineEmits<{ 'open-settings': [] }>()
</script>

<template>
  <div data-no-active class="shrink-0 border-b border-border bg-warning-soft px-4 py-3 sm:px-8 lg:px-12">
    <div class="flex items-center gap-3">
      <CircleAlert class="size-4 shrink-0 text-warning" :stroke-width="1.5" />
      <p class="text-meta text-ink">还没有激活的服务商。去服务商设置点一下行尾的 ⏻，就能开始聊天。</p>
      <button
        type="button"
        data-open-settings-list
        class="ml-auto h-8 shrink-0 rounded-md border border-border-strong bg-surface px-3 text-meta font-medium text-ink transition-colors duration-150 hover:bg-surface-alt"
        @click="$emit('open-settings')"
      >
        去选择
      </button>
    </div>
  </div>
</template>
