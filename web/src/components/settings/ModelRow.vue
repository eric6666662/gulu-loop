<script setup lang="ts">
import { Trash2 } from 'lucide-vue-next'
import type { ModelRef } from '../../api/providers'

const props = defineProps<{ model: ModelRef; isDefault: boolean }>()
const emit = defineEmits<{ update: [model: ModelRef]; remove: []; 'set-default': [] }>()

function patch(part: Partial<ModelRef>): void {
  emit('update', { ...props.model, ...part })
}
</script>

<template>
  <div class="rounded-md border border-border bg-surface px-3 py-2">
    <div class="flex items-center gap-2">
      <label
        class="flex shrink-0 items-center gap-2 text-meta"
        :class="isDefault ? 'text-ink' : 'text-ink-muted'"
      >
        <input
          type="radio"
          name="provider-default-model"
          data-set-default
          class="size-4 accent-primary"
          :checked="isDefault"
          @change="emit('set-default')"
        />
        默认
      </label>

      <input
        data-model-id
        type="text"
        aria-label="模型标识"
        :value="model.id"
        class="block h-9 min-w-0 flex-1 rounded-md border border-border-strong bg-surface px-3 font-mono text-meta text-ink transition-colors duration-150 hover:border-ink-subtle focus:border-primary"
        @input="patch({ id: ($event.target as HTMLInputElement).value })"
      />
      <input
        data-model-name
        type="text"
        aria-label="模型展示名"
        placeholder="展示名（可选）"
        :value="model.name"
        class="block h-9 min-w-0 flex-1 rounded-md border border-border-strong bg-surface px-3 text-body text-ink transition-colors duration-150 hover:border-ink-subtle focus:border-primary"
        @input="patch({ name: ($event.target as HTMLInputElement).value })"
      />

      <button
        v-if="!isDefault"
        type="button"
        data-model-remove
        :aria-label="`删除模型 ${model.id}`"
        class="flex size-7 shrink-0 items-center justify-center rounded-sm text-ink-muted transition-colors duration-150 hover:bg-danger-soft hover:text-danger"
        @click="emit('remove')"
      >
        <Trash2 class="size-4" :stroke-width="1.5" />
      </button>
    </div>
  </div>
</template>
