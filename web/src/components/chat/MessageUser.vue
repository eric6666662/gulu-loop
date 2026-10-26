<script setup lang="ts">
import type { Message } from '../../api/messages'

// 用户消息：整页唯一带底色的内容块（02-ui设计.md MessageUser）
defineProps<{ message: Message }>()
</script>

<template>
  <div data-message class="group flex justify-end">
    <div class="flex max-w-[85%] flex-col items-end">
      <div class="rounded-xl rounded-tr-xs bg-surface-alt px-4 py-3 text-body-lg text-ink whitespace-pre-wrap">
        {{ message.content }}
      </div>
    </div>
  </div>
</template>
