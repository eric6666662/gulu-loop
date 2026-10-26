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
const emit = defineEmits<{ retry: []; 'focus-composer': []; 'open-settings': [] }>()

// 后端按 seq 倒序返回，渲染前按 seq 升序（02-后端设计.md #6）
const ordered = computed(() => [...props.messages].sort((a, b) => a.seq - b.seq))

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
      <template v-for="m in ordered" :key="m.id">
        <MessageUser v-if="m.role === 'user'" :message="m" />
        <MessageAssistant v-else-if="m.role === 'assistant'" :message="m" />
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
