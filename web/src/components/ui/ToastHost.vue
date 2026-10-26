<script setup lang="ts">
import { CircleAlert, Check } from 'lucide-vue-next'
import { useToast } from '../../composables/useToast'

const { toasts } = useToast()
</script>

<template>
  <div class="pointer-events-none fixed bottom-6 left-1/2 z-50 flex -translate-x-1/2 flex-col gap-2">
    <div
      v-for="t in toasts"
      :key="t.id"
      role="status"
      class="flex items-center gap-2 rounded-md px-3 py-2 text-body font-medium text-on-primary shadow-overlay"
      :class="t.tone === 'danger' ? 'bg-danger' : 'bg-ink'"
    >
      <CircleAlert v-if="t.tone === 'danger'" class="size-4 shrink-0" :stroke-width="1.5" />
      <Check v-else class="size-4 shrink-0" :stroke-width="1.5" />
      <span>{{ t.text }}</span>
    </div>
  </div>
</template>
