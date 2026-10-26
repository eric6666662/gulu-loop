<script setup lang="ts">
import type { Provider } from '../../api/providers'
import ProviderRow from './ProviderRow.vue'

defineProps<{ providers: Provider[] }>()
defineEmits<{ edit: [provider: Provider]; test: [provider: Provider]; remove: [provider: Provider] }>()
</script>

<template>
  <ul class="mx-auto flex w-full max-w-wide flex-col gap-2 px-4 pb-6 sm:px-8 lg:px-12" data-provider-list>
    <ProviderRow
      v-for="p in providers"
      :key="p.serviceProvider"
      :provider="p"
      @edit="$emit('edit', p)"
      @test="$emit('test', p)"
      @remove="$emit('remove', p)"
    />
  </ul>
</template>
