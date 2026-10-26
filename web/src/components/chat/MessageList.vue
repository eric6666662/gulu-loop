<script setup lang="ts">
import { computed } from 'vue'
import type { Message } from '../../api/messages'
import type { ChatStatus } from '../../composables/useChat'
import MessageAssistant from './MessageAssistant.vue'
import MessageUser from './MessageUser.vue'
import SkeletonList from './SkeletonList.vue'
import EmptyState from './EmptyState.vue'
import StreamError from './StreamError.vue'

const props = defineProps<{
  messages: Message[]
  status: ChatStatus
  error: string
  errorKind?: 'load' | 'stream' | ''
  draft?: boolean
}>()
const emit = defineEmits<{
  /** 消息级重试（截断重发），与下面 StreamError 的 retry（重拉历史）区分开 */
  'message-retry': [message: Message]
  edit: [message: Message]
  copy: [message: Message]
  /** StreamError 的「重试」= 重新拉历史 */
  retry: []
  'focus-composer': []
  'open-settings': []
}>()

// 后端按 seq 倒序返回，渲染前按 seq 升序（02-后端设计.md #6）
const ordered = computed(() => [...props.messages].sort((a, b) => a.seq - b.seq))

/**
 * 渲染行：消息 + 换模型痕迹。
 *
 * 痕迹是**派生**的，不额外存状态：相邻两条 assistant 的 model 不同，就在后一条前面
 * 插一条「已切换到 xxx」（02-ui设计.md ModelSwitchNotice：仅真实切换时出现）。
 */
type Row = { kind: 'message'; message: Message } | { kind: 'notice'; model: string; key: string }

const rows = computed<Row[]>(() => {
  const out: Row[] = []
  let prevModel = ''
  for (const m of ordered.value) {
    if (m.role === 'assistant') {
      if (prevModel && m.model && m.model !== prevModel) {
        out.push({ kind: 'notice', model: m.model, key: `notice-${m.id}` })
      }
      if (m.model) prevModel = m.model
    }
    out.push({ kind: 'message', message: m })
  }
  return out
})

// 两种错误文案（02-ui设计.md StreamError）：拉历史失败 vs 生成中失败
const errorTitle = computed(() => {
  if (props.errorKind === 'stream') {
    const model = [...ordered.value].reverse().find((m) => m.role === 'assistant')?.model
    return model ? `${model} 调用失败：${props.error}` : `调用失败：${props.error}`
  }
  return `消息没加载出来：${props.error}`
})
const errorHint = computed(() =>
  props.errorKind === 'stream'
    ? '到「服务商设置」检查配置后点重试。这次回复没有写入会话。'
    : '检查本地服务是否还在跑，然后点重试。',
)
</script>

<template>
  <div class="w-full px-4 py-6 sm:px-8 lg:px-12">
    <SkeletonList v-if="status === 'loading' && ordered.length === 0" />

    <EmptyState
      v-else-if="ordered.length === 0 && status !== 'error'"
      :draft="draft"
      @focus-composer="emit('focus-composer')"
    />

    <template v-else>
      <template v-for="row in rows" :key="row.kind === 'message' ? row.message.id : row.key">
        <div v-if="row.kind === 'notice'" data-model-switch-notice class="my-8 flex items-center gap-3">
          <span class="h-px flex-1 bg-border"></span>
          <span class="text-caption text-ink-muted">已切换到 {{ row.model }}</span>
          <span class="h-px flex-1 bg-border"></span>
        </div>
        <MessageUser
          v-else-if="row.message.role === 'user'"
          :message="row.message"
          @edit="emit('edit', row.message)"
          @copy="emit('copy', row.message)"
        />
        <MessageAssistant
          v-else-if="row.message.role === 'assistant'"
          :message="row.message"
          @retry="emit('message-retry', row.message)"
          @copy="emit('copy', row.message)"
        />
      </template>
    </template>

    <StreamError
      v-if="status === 'error'"
      :title="errorTitle"
      :message="errorHint"
      @retry="emit('retry')"
      @open-settings="emit('open-settings')"
    />
  </div>
</template>
