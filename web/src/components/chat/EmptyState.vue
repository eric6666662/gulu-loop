<script setup lang="ts">
// 空会话：行动邀请，不是"暂无数据"（docs/设计稿.md §8）
defineEmits<{ 'focus-composer': [] }>()
</script>

<template>
  <div class="mx-auto w-full max-w-form px-6 pt-12 text-center">
    <h2 class="text-title font-semibold text-ink">这个会话还是空的</h2>
    <p class="mt-2 text-body text-ink-muted">在下方输入框写第一句，回车发送。</p>
    <button
      type="button"
      data-focus-composer
      class="mt-6 inline-flex h-9 items-center rounded-md bg-primary px-4 text-body font-medium text-on-primary transition-colors duration-150 hover:bg-primary-hover"
      @click="$emit('focus-composer')"
    >
      写第一句
    </button>
  </div>
</template>
