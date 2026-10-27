<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import ChatHeader from './ChatHeader.vue'
import MessageList from './MessageList.vue'
import ProviderBanner from './ProviderBanner.vue'
import Composer from './Composer.vue'
import EditMessageDialog from '../dialog/EditMessageDialog.vue'
import { useChat } from '../../composables/useChat'
import type { UseModelSelection } from '../../composables/useModelSelection'
import type { UseProviders } from '../../composables/useProviders'
import type { Message } from '../../api/messages'
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

// 服务商数据从 App provide 下来（同一实例）：激活一变，模型回落 watch 自动触发，
// 不需要事件广播（02-前端设计.md §3）
const providers = inject('providers') as UseProviders
// 选中模型是**跨视图**状态（App 持有）：切到设置页再回来不能丢（ADR-0016 修正）
const selection = inject('modelSelection') as UseModelSelection

const { messages, status, error, errorKind, refresh, send, retry, edit, stop } = useChat(conversationId, {
  ensureConversation: () => props.ensureConversation?.() ?? Promise.reject(new Error('缺少建会话能力')),
  onConversationReady: (id) => emit('conversation-ready', id),
  onDone: (id, count) => emit('done', id, count),
  // 每次请求前读当前选中的 (serviceProvider, model)（ADR-0016）
  getModel: () => selection.selected.value,
})

const composer = ref<InstanceType<typeof Composer> | null>(null)

/** 回落痕迹的展示形态：{服务商, 新模型}；没有回落就是 null */
const fallbackNotice = computed(() => {
  const f = selection.fallback.value
  const p = providers.activeProvider.value
  if (!f || !p) return null
  return { provider: p.serviceProvider, model: f.to }
})

/** 选中某个模型：值带服务商身份（同名模型因此不再歧义） */
function onPickModel(id: string): void {
  const m = selection.models.value.find((x) => x.id === id)
  if (!m) return
  selection.select({ serviceProvider: m.serviceProvider, model: m.id })
}

async function onSend(text: string): Promise<void> {
  // 回落痕迹只留到下一轮（02-前端设计.md §6：不落库、不堆积）
  selection.clearFallback()
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

    <ProviderBanner v-if="canSend === false" @open-settings="emit('open-settings')" />

    <div class="scroll-thin min-h-0 flex-1 overflow-y-auto">
      <MessageList
        :messages="messages"
        :status="status"
        :error="error"
        :error-kind="errorKind"
      :fallback="fallbackNotice"
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
      :models="selection.models.value"
      :model="selection.label.value"
      :provider="providers.activeProvider.value?.serviceProvider ?? ''"
      @send="onSend"
      @stop="stop"
      @update:model="onPickModel"
    />
  </div>
</template>
