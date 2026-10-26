<script setup lang="ts">
import { nextTick, onMounted, ref, watch } from 'vue'
import type { Conversation } from '../../api/conversations'
import ConversationItem from './ConversationItem.vue'
import ListLoadingMore from './ListLoadingMore.vue'

const props = defineProps<{
  items: Conversation[]
  activeId: string
  hasMore: boolean
  loading: boolean
}>()
const emit = defineEmits<{ select: [id: string]; delete: [conversation: Conversation]; 'load-more': [] }>()

const scroller = ref<HTMLElement | null>(null)

function maybeLoadMore(): void {
  const el = scroller.value
  if (!el || !props.hasMore || props.loading) return
  if (el.scrollTop + el.clientHeight >= el.scrollHeight - 24) emit('load-more')
}

/** 首屏 10 条没填满容器时自动补一批，避免"有下一页却没有滚动条"（02-ui设计.md 流程 8） */
async function fillViewport(): Promise<void> {
  await nextTick()
  const el = scroller.value
  if (!el || !props.hasMore || props.loading) return
  if (el.scrollHeight <= el.clientHeight + 8) emit('load-more')
}

onMounted(fillViewport)
watch(() => props.items.length, fillViewport)
</script>

<template>
  <nav ref="scroller" class="scroll-thin min-h-0 flex-1 overflow-y-auto px-2" aria-label="会话列表" @scroll="maybeLoadMore">
    <p v-if="items.length === 0 && !loading" class="px-3 py-6 text-caption text-ink-muted">
      还没有会话 / 点下方的「新的聊天」，或者直接在输入框写第一句。
    </p>

    <div class="flex flex-col gap-1">
      <ConversationItem
        v-for="c in items"
        :key="c.id"
        :conversation="c"
        :active="c.id === activeId"
        @select="emit('select', c.id)"
        @delete="emit('delete', c)"
      />
    </div>

    <ListLoadingMore v-if="loading" />
  </nav>
</template>
