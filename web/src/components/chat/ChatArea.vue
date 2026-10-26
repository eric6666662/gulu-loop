<script setup lang="ts">
import ChatHeader from './ChatHeader.vue'

// 任务 4：会话头可用（标题 + 重命名 + 窄屏抽屉入口）；
// 消息区与输入区由任务 5、6 接入。
defineProps<{ title: string }>()
const emit = defineEmits<{ rename: []; 'open-sidebar': [] }>()
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <ChatHeader :title="title" @rename="emit('rename')" @open-sidebar="emit('open-sidebar')" />
    <div class="scroll-thin min-h-0 flex-1 overflow-y-auto"></div>
  </div>
</template>
