<script setup lang="ts">
import { computed } from 'vue'
import type { Message } from '../../api/messages'
import type { ChatStatus } from '../../composables/useChat'
import MessageAssistant from './MessageAssistant.vue'
import MessageUser from './MessageUser.vue'
import SkeletonList from './SkeletonList.vue'
import EmptyState from './EmptyState.vue'
import StreamError from './StreamError.vue'

const props = defineProps<{ messages: Message[]; status: ChatStatus; error: string }>()
const emit = defineEmits<{ retry: []; 'focus-composer': []; 'open-settings': [] }>()

// 后端按 seq 倒序返回，渲染前按 seq 升序（02-后端设计.md #6）
const ordered = computed(() => [...props.messages].sort((a, b) => a.seq - b.seq))
</script>

<template>
  <div class="w-full px-4 py-6 sm:px-8 lg:px-12">
    <SkeletonList v-if="status === 'loading' && ordered.length === 0" />

    <EmptyState v-else-if="ordered.length === 0 && status !== 'error'" @focus-composer="emit('focus-composer')" />

    <template v-else>
      <template v-for="m in ordered" :key="m.id">
        <MessageUser v-if="m.role === 'user'" :message="m" />
        <MessageAssistant v-else-if="m.role === 'assistant'" :message="m" />
      </template>
    </template>

    <StreamError
      v-if="status === 'error'"
      :title="`消息没加载出来：${error}`"
      message="检查本地服务是否还在跑，然后点重试。"
      @retry="emit('retry')"
      @open-settings="emit('open-settings')"
    />
  </div>
</template>
