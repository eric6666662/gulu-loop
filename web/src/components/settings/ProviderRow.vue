<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Ellipsis, Link, Pencil } from 'lucide-vue-next'
import type { Provider } from '../../api/providers'
import ProviderAvatar from './ProviderAvatar.vue'
import ProviderRowMenu from './ProviderRowMenu.vue'

const props = defineProps<{ provider: Provider }>()
const emit = defineEmits<{ edit: []; test: []; remove: [] }>()

// 抽屉状态归本组件：菜单绝对定位在行内，点外部 / Esc 收起（02-ui设计.md ProviderRowMenu）
const menuOpen = ref(false)

function toggleMenu(): void {
  menuOpen.value = !menuOpen.value
}
function closeMenu(): void {
  menuOpen.value = false
}
function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') closeMenu()
}
function onDocClick(e: MouseEvent): void {
  const el = e.target as HTMLElement | null
  if (!el?.closest('[data-menu-root]')) closeMenu()
}

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  document.addEventListener('click', onDocClick)
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown)
  document.removeEventListener('click', onDocClick)
})

// 列表里只显示域名（去掉协议头与末尾斜杠），完整 URL 留给 <a href>
const websiteLabel = computed(() =>
  (props.provider.website ?? '').replace(/^https?:\/\//i, '').replace(/\/+$/, ''),
)
</script>

<template>
  <li class="rounded-md border border-border bg-surface">
    <div class="flex items-center gap-3 px-3 py-3">
      <ProviderAvatar :name="provider.serviceProvider" />

      <div class="min-w-0 flex-1">
        <div class="flex items-center gap-2">
          <span class="truncate text-body font-medium text-ink">{{ provider.serviceProvider }}</span>
          <span class="shrink-0 text-caption text-ink-muted">{{ provider.models.length }} 个模型</span>
        </div>
        <a
          v-if="websiteLabel"
          data-website
          :href="provider.website"
          target="_blank"
          rel="noopener noreferrer"
          class="mt-1 flex items-center gap-1 text-caption text-ink-muted transition-colors duration-150 hover:text-ink"
        >
          <Link class="size-3.5 shrink-0" :stroke-width="1.5" />
          <span class="truncate">{{ websiteLabel }}</span>
        </a>
      </div>

      <button
        type="button"
        data-edit
        :aria-label="`编辑 ${provider.serviceProvider}`"
        class="flex size-7 shrink-0 items-center justify-center rounded-sm text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
        @click="$emit('edit')"
      >
        <Pencil class="size-4" :stroke-width="1.5" />
      </button>

      <div class="relative shrink-0" data-menu-root>
        <button
          type="button"
          data-menu
          aria-haspopup="menu"
          :aria-expanded="menuOpen ? 'true' : 'false'"
          :aria-label="`${provider.serviceProvider} 的更多操作`"
          class="flex size-7 items-center justify-center rounded-sm text-ink-muted transition-colors duration-150 hover:bg-surface-alt hover:text-ink"
          @click.stop="toggleMenu"
        >
          <Ellipsis class="size-4" :stroke-width="1.5" />
        </button>
        <ProviderRowMenu
          v-if="menuOpen"
          @test="closeMenu(), emit('test')"
          @remove="closeMenu(), emit('remove')"
        />
      </div>
    </div>
  </li>
</template>
