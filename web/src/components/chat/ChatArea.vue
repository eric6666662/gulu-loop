<script setup lang="ts">
import { computed } from 'vue'
import ChatHeader from './ChatHeader.vue'
import MessageList from './MessageList.vue'
import { useChat } from '../../composables/useChat'

// 任务 5：会话头 + 消息区（历史渲染 / 骨架 / 空态 / 错误卡）。
// 输入区与发送在任务 6 接入。
const props = defineProps<{ conversationId: string; title: string }>()
const emit = defineEmits<{ rename: []; 'open-sidebar': []; 'open-settings': [] }>()

const conversationId = computed(() => props.conversationId)
const { messages, status, error, refresh } = useChat(conversationId)
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <ChatHeader :title="title" @rename="emit('rename')" @open-sidebar="emit('open-sidebar')" />

    <div class="scroll-thin min-h-0 flex-1 overflow-y-auto">
      <MessageList
        :messages="messages"
        :status="status"
        :error="error"
        @retry="refresh"
        @focus-composer="emit('open-settings')"
        @open-settings="emit('open-settings')"
      />
    </div>
  </div>
</template>
