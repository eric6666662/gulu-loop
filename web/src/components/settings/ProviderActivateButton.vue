<script setup lang="ts">
import { Power } from 'lucide-vue-next'

// 行尾激活按钮（02-ui设计.md §3 ProviderActivateButton）：
// 未激活＝纯图标；已激活＝图标 + 文字「激活」（primary-soft 底）。
// 已激活时再点不 emit —— 它是状态，不是可重复执行的动作（幂等）。
const props = defineProps<{ name: string; active: boolean }>()
const emit = defineEmits<{ activate: [] }>()

function onClick(): void {
  if (props.active) return
  emit('activate')
}
</script>

<template>
  <button
    type="button"
    data-activate
    :aria-pressed="active ? 'true' : 'false'"
    :aria-label="active ? `已激活 ${name}` : `激活 ${name}`"
    :class="
      active
        ? 'flex h-7 shrink-0 items-center justify-center gap-1.5 rounded-sm bg-primary-soft px-2 text-meta font-medium text-primary'
        : 'flex size-7 shrink-0 items-center justify-center rounded-sm text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink'
    "
    @click="onClick"
  >
    <Power class="size-4 shrink-0" :stroke-width="1.5" />
    <span v-if="active" data-active-label class="text-meta">激活</span>
  </button>
</template>
