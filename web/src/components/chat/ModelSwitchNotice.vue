<script setup lang="ts">
// 换模型痕迹（02-ui设计.md §3 ModelSwitchNotice）：一条发丝线 + 居中说明。
//
// 三种文案，靠 props 区分，不给调用方拼字符串：
//   reason='switch'（默认）+ 无 provider → 「已切换到 deepseek-reasoner」
//   reason='switch' + provider         → 「已切换到 中转站 · deepseek-chat」（换服务商但模型名相同时才需要）
//   reason='fallback'                  → 「服务商换成 X，模型跟着换成 Y」（主语是系统，不是用户主动换的）
defineProps<{ model: string; provider?: string; reason?: 'switch' | 'fallback' }>()
</script>

<template>
  <div data-model-switch-notice class="my-8 flex items-center gap-3">
    <span class="h-px flex-1 bg-border"></span>
    <span class="text-caption text-ink-muted">
      <template v-if="reason === 'fallback'">服务商换成 {{ provider }}，模型跟着换成 {{ model }}</template>
      <template v-else-if="provider">已切换到 {{ provider }} · {{ model }}</template>
      <template v-else>已切换到 {{ model }}</template>
    </span>
    <span class="h-px flex-1 bg-border"></span>
  </div>
</template>
