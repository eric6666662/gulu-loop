<script setup lang="ts">
import { computed } from 'vue'

// 首字母方块：MVP 不做图标上传（02-ui设计.md 待确认点 15）
const props = defineProps<{ name: string }>()

// 中文取首字、英文取首字母大写
const initial = computed(() => (props.name.trim()[0] ?? '?').toUpperCase())
</script>

<template>
  <span
    class="flex size-8 shrink-0 items-center justify-center rounded-md bg-surface-alt text-body font-semibold text-ink"
    aria-hidden="true"
    >{{ initial }}</span
  >
</template>
