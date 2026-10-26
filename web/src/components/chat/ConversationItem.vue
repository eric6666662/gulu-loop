<script setup lang="ts">
import { computed } from 'vue'
import { X } from 'lucide-vue-next'
import type { Conversation } from '../../api/conversations'
import { messageCount, relativeTime } from '../../lib/format'

const props = defineProps<{ conversation: Conversation; active: boolean }>()
defineEmits<{ select: []; delete: [] }>()

const title = computed(() => props.conversation.title.trim() || '新会话')
const count = computed(() => messageCount(props.conversation.messageCount))
const time = computed(() => relativeTime(props.conversation.lastMessageTime))
</script>

<template>
  <div class="group relative">
    <a
      href="#"
      data-conv
      :aria-current="active ? 'true' : undefined"
      class="block rounded-md border-l-2 py-2 pr-8 pl-3 transition-colors duration-150 hover:bg-surface"
      :class="active ? 'border-primary bg-surface' : 'border-transparent'"
      @click.prevent="$emit('select')"
    >
      <span class="block truncate text-body text-ink" :class="active ? 'font-medium' : ''">{{ title }}</span>
      <span class="mt-1 flex items-baseline justify-between gap-2 text-caption text-ink-muted">
        <span data-numeric>{{ count }}</span>
        <time data-numeric>{{ time }}</time>
      </span>
    </a>
    <button
      type="button"
      data-conv-delete
      :aria-label="`删除会话：${title}`"
      class="absolute top-2 right-1 flex size-6 items-center justify-center rounded-sm text-ink-muted opacity-0 transition-opacity duration-150 hover:bg-surface-alt hover:text-danger focus-visible:opacity-100 group-hover:opacity-100"
      @click.stop="$emit('delete')"
    >
      <X class="size-4" :stroke-width="1.5" />
    </button>
  </div>
</template>
