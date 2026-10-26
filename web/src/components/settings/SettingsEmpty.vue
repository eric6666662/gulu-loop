<script setup lang="ts">
import { Plus } from 'lucide-vue-next'

defineEmits<{ create: [] }>()
</script>

<template>
  <div class="flex flex-col items-center justify-center px-4 py-12 text-center">
    <h3 class="text-title font-semibold text-ink">还没有服务商</h3>
    <p class="mt-2 text-body text-ink-muted">
      添加第一个服务商：填 base_url 和 api_key，点「拉取模型列表」挑一个模型，就能开始对话。
    </p>
    <button
      type="button"
      class="mt-6 inline-flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-body font-medium text-on-primary transition-colors duration-150 hover:bg-primary-hover"
      @click="$emit('create')"
    >
      <Plus class="size-4" :stroke-width="1.5" />添加服务商
    </button>
  </div>
</template>
