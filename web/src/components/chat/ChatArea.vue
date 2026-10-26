<script setup lang="ts">
import { computed, ref } from 'vue'
import ChatHeader from './ChatHeader.vue'
import MessageList from './MessageList.vue'
import Composer from './Composer.vue'
import EditMessageDialog from '../dialog/EditMessageDialog.vue'
import { useChat } from '../../composables/useChat'
import type { Message } from '../../api/messages'

// 会话头 + 消息区 + 输入区。草稿态（conversationId === ''）走默认对话页（01-需求 #5b）。
const props = defineProps<{
  conversationId: string
  title: string
  /** 草稿态首次发送时由 App 注入：先建会话再发 */
  ensureConversation?: () => Promise<string>
  /** 有没有可用服务商（没有时禁止发送） */
  canSend?: boolean
}>()
const emit = defineEmits<{
  rename: []
  'open-sidebar': []
  'open-settings': []
  'conversation-ready': [id: string]
  done: [conversationId: string, messageCount: number]
}>()

const conversationId = computed(() => props.conversationId)
const isDraft = computed(() => props.conversationId === '')

const { messages, status, error, errorKind, refresh, send, retry, edit, stop } = useChat(conversationId, {
  ensureConversation: () => props.ensureConversation?.() ?? Promise.reject(new Error('缺少建会话能力')),
  onConversationReady: (id) => emit('conversation-ready', id),
  onDone: (id, count) => emit('done', id, count),
})

const composer = ref<InstanceType<typeof Composer> | null>(null)

async function onSend(text: string): Promise<void> {
  await send(text)
}

// ---- 重试 / 编辑后重发（ADR-0006）----
const editing = ref<Message | null>(null)

function askEdit(message: Message): void {
  editing.value = message
}

async function onEditSave(content: string): Promise<void> {
  const target = editing.value
  editing.value = null
  if (target) await edit(target.id, content)
}

function onRetry(message: Message): void {
  void retry(message.id)
}

defineExpose({
  focusComposer: () => composer.value?.focus(),
  clearComposer: () => composer.value?.clear(),
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <ChatHeader :title="title" @rename="emit('rename')" @open-sidebar="emit('open-sidebar')" />

    <div class="scroll-thin min-h-0 flex-1 overflow-y-auto">
      <MessageList
        :messages="messages"
        :status="status"
        :error="error"
        :error-kind="errorKind"
        :draft="isDraft"
        @retry="refresh"
        @message-retry="onRetry"
        @edit="askEdit"
        @focus-composer="composer?.focus()"
        @open-settings="emit('open-settings')"
      />
    </div>

    <EditMessageDialog
      :open="editing !== null"
      :initial-content="editing?.content ?? ''"
      @save="onEditSave"
      @cancel="editing = null"
    />

    <Composer
      ref="composer"
      :streaming="status === 'streaming'"
      :disabled="canSend === false"
      @send="onSend"
      @stop="stop"
    />
  </div>
</template>
