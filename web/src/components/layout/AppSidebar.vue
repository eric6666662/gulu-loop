<script setup lang="ts">
import { Plus, Settings, X } from 'lucide-vue-next'
import type { Conversation } from '../../api/conversations'
import ConversationList from '../chat/ConversationList.vue'

const props = defineProps<{
  items: Conversation[]
  activeId: string
  hasMore: boolean
  loading: boolean
  open: boolean
}>()
const emit = defineEmits<{
  select: [id: string]
  delete: [conversation: Conversation]
  'new-chat': []
  'open-settings': []
  'load-more': []
  close: []
}>()
</script>

<template>
  <div
    v-if="props.open"
    class="fixed inset-0 z-30 bg-overlay lg:hidden"
    data-sidebar-scrim
    @click="emit('close')"
  ></div>

  <aside
    class="fixed inset-y-0 left-0 z-40 flex w-64 shrink-0 flex-col border-r border-border bg-surface-alt transition-transform duration-150 ease-out-soft lg:static lg:z-auto lg:w-56 lg:translate-x-0"
    :class="props.open ? 'translate-x-0' : '-translate-x-full'"
    aria-label="会话栏"
  >
    <div class="flex h-14 shrink-0 items-center pr-2 pl-3">
      <span class="text-body font-semibold tracking-tight text-ink">咕噜</span>
      <button
        type="button"
        data-sidebar-close
        class="ml-auto flex size-8 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-surface hover:text-ink lg:hidden"
        aria-label="收起会话栏"
        @click="emit('close')"
      >
        <X class="size-4" :stroke-width="1.5" />
      </button>
    </div>

    <ConversationList
      :items="props.items"
      :active-id="props.activeId"
      :has-more="props.hasMore"
      :loading="props.loading"
      @select="emit('select', $event)"
      @delete="emit('delete', $event)"
      @load-more="emit('load-more')"
    />

    <div class="flex shrink-0 items-center gap-2 border-t border-border p-2">
      <button
        type="button"
        data-open-settings
        class="flex size-8 items-center justify-center rounded-md text-ink-muted transition-colors duration-150 hover:bg-surface hover:text-ink"
        aria-label="服务商设置"
        @click="emit('open-settings')"
      >
        <Settings class="size-4" :stroke-width="1.5" />
      </button>
      <button
        type="button"
        data-new-chat
        class="ml-auto flex h-8 items-center gap-2 rounded-md bg-primary px-3 text-meta font-medium text-on-primary transition-colors duration-150 hover:bg-primary-hover"
        @click="emit('new-chat')"
      >
        <Plus class="size-4" :stroke-width="1.5" />新的聊天
      </button>
    </div>
  </aside>
</template>
