<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import ChatHeader from './ChatHeader.vue'
import MessageList from './MessageList.vue'
import Composer from './Composer.vue'
import EditMessageDialog from '../dialog/EditMessageDialog.vue'
import { useChat } from '../../composables/useChat'
import type { Message } from '../../api/messages'
import type { ProviderModel } from '../../composables/useProviders'
import * as conversationApi from '../../api/conversations'
import * as messageApi from '../../api/messages'
import { downloadText } from '../../lib/download'
import { useToast } from '../../composables/useToast'

// 会话头 + 消息区 + 输入区。草稿态（conversationId === ''）走默认对话页（01-需求 #5b）。
const props = defineProps<{
  conversationId: string
  title: string
  /** 草稿态首次发送时由 App 注入：先建会话再发 */
  ensureConversation?: () => Promise<string>
  /** 有没有可用服务商（没有时禁止发送） */
  canSend?: boolean
  /** 可选模型（含所属服务商） */
  models?: ProviderModel[]
  /** 默认模型（服务商配置里的默认值） */
  defaultModel?: string
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

const { messages, model, setModel, status, error, errorKind, refresh, send, retry, edit, stop } = useChat(conversationId, {
  ensureConversation: () => props.ensureConversation?.() ?? Promise.reject(new Error('缺少建会话能力')),
  onConversationReady: (id) => emit('conversation-ready', id),
  onDone: (id, count) => emit('done', id, count),
})

// 切会话 → 回落到默认模型；默认模型后到时（服务商列表异步加载）也补上
watch(() => props.conversationId, () => setModel(props.defaultModel ?? ''), { immediate: true })
watch(
  () => props.defaultModel,
  (def) => {
    if (!model.value && def) setModel(def)
  },
)

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

const { push } = useToast()

/** 分享 = 导出 Markdown 并下载（后端返回 JSON，前端造 Blob） */
async function onExport(): Promise<void> {
  if (!props.conversationId) {
    push('先发一句话再导出', 'danger')
    return
  }
  try {
    const { filename, content } = await conversationApi.exportMarkdown(props.conversationId)
    downloadText(filename, content)
    push(`已导出：${filename}`)
  } catch (e) {
    push(`导出失败：${e instanceof Error ? e.message : String(e)}`, 'danger')
  }
}

/** 复制单条消息的纯文本（走 #8 单条内容接口，不用渲染后的 HTML） */
async function onCopy(message: Message): Promise<void> {
  try {
    const text = await messageApi.content(props.conversationId, message.id)
    await navigator.clipboard.writeText(text)
    push('已复制')
  } catch {
    push('复制失败：剪贴板不可用', 'danger')
  }
}

defineExpose({
  focusComposer: () => composer.value?.focus(),
  clearComposer: () => composer.value?.clear(),
})
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <ChatHeader
      :title="title"
      @refresh="refresh"
      @rename="emit('rename')"
      @export="onExport"
      @open-sidebar="emit('open-sidebar')"
    />

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
        @copy="onCopy"
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
      :models="models ?? []"
      :model="model"
      @send="onSend"
      @stop="stop"
      @update:model="setModel"
    />
  </div>
</template>
