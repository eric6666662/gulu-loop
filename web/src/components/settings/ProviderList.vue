<script setup lang="ts">
import { computed, inject } from 'vue'
import { CircleAlert } from 'lucide-vue-next'
import type { Provider } from '../../api/providers'
import type { UseProviders } from '../../composables/useProviders'
import ProviderRow from './ProviderRow.vue'

const props = defineProps<{ providers: Provider[] }>()
defineEmits<{
  activate: [provider: Provider]
  edit: [provider: Provider]
  test: [provider: Provider]
  remove: [provider: Provider]
}>()

const allProviders = inject('providers') as UseProviders
// 有服务商但一个都没激活（01-需求 #1f）：提示用户去点行尾的激活按钮。
// **不是独立组件** —— 无 props / 无 emit / 无状态，拆出来只是一层转发（02-前端设计.md §2.2）。
const noActive = computed(() => props.providers.length > 0 && allProviders.activeProvider.value === null)
</script>

<template>
  <div>
    <div
      v-if="noActive"
      data-settings-noactive
      class="shrink-0 border-b border-border bg-warning-soft px-4 py-3 sm:px-8 lg:px-12"
    >
      <div class="flex items-center gap-3">
        <CircleAlert class="size-4 shrink-0 text-warning" :stroke-width="1.5" />
        <p class="text-meta text-ink">还没有激活的服务商。点行尾的 ⏻ 激活一个，聊天页才能发送消息。</p>
      </div>
    </div>

    <ul class="mx-auto flex w-full max-w-wide flex-col gap-2 px-4 pb-6 sm:px-8 lg:px-12" data-provider-list>
      <ProviderRow
        v-for="p in providers"
        :key="p.serviceProvider"
        :provider="p"
        @activate="$emit('activate', p)"
        @edit="$emit('edit', p)"
        @test="$emit('test', p)"
        @remove="$emit('remove', p)"
      />
    </ul>
  </div>
</template>
