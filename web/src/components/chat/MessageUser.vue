<script setup lang="ts">
import { Pencil } from 'lucide-vue-next'
import type { Message } from '../../api/messages'

// 用户消息：整页唯一带底色的内容块（02-ui设计.md MessageUser）
defineProps<{ message: Message }>()
const emit = defineEmits<{ edit: [] }>()
</script>

<template>
  <div data-message class="group flex justify-end">
    <div class="flex max-w-[85%] flex-col items-end">
      <div class="rounded-xl rounded-tr-xs bg-surface-alt px-4 py-3 text-body-lg text-ink whitespace-pre-wrap">
        {{ message.content }}
      </div>
      <!-- 操作组：hover / focus-within 才出现（复制在任务 9） -->
      <div
        class="mt-1 flex items-center gap-1 opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100"
      >
        <button
          type="button"
          data-edit
          aria-label="编辑这条消息"
          class="flex size-7 items-center justify-center rounded-sm text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
          @click="emit('edit')"
        >
          <Pencil class="size-4" :stroke-width="1.5" />
        </button>
      </div>
    </div>
  </div>
</template>
