<script setup lang="ts">
import { computed, onMounted, provide, ref } from 'vue'
import AppSidebar from './components/layout/AppSidebar.vue'
import ChatArea from './components/chat/ChatArea.vue'
import SettingsView from './components/settings/SettingsView.vue'
import ConfirmDialog from './components/dialog/ConfirmDialog.vue'
import RenameDialog from './components/dialog/RenameDialog.vue'
import ToastHost from './components/ui/ToastHost.vue'
import type { Conversation } from './api/conversations'
import { useConversationList } from './composables/useConversationList'
import { useProviders } from './composables/useProviders'
import { useSelectedFromQuery } from './composables/useSelectedFromQuery'
import { useSettingsPane } from './composables/useSettingsPane'
import { useToast } from './composables/useToast'

const providers = useProviders()
const settingsPane = useSettingsPane()
const list = useConversationList()
const { selectedId, select } = useSelectedFromQuery(list.items)
const { push } = useToast()

provide('providers', providers)
provide('settingsPane', settingsPane)

const sidebarOpen = ref(false)
const currentTitle = computed(
  () => list.items.value.find((c) => c.id === selectedId.value)?.title ?? '',
)

onMounted(() => {
  // 两块数据都要在启动时加载：会话列表给侧栏，服务商列表给设置页与模型切换器。
  // 曾经只调了 loadFirstPage，导致进设置页时服务商列表是空的（2026-10-26 修复）。
  void list.loadFirstPage()
  void providers.load()
})

function message(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

async function onNewChat(): Promise<void> {
  try {
    const created = await list.create()
    select(created.id)
    push('已新建会话')
    sidebarOpen.value = false
  } catch (e) {
    push(`新建失败：${message(e)}`, 'danger')
  }
}

// ---- 删除会话（确认弹层）----
const pendingDelete = ref<Conversation | null>(null)
const confirmOpen = ref(false)

function askDelete(c: Conversation): void {
  pendingDelete.value = c
  confirmOpen.value = true
}

async function doDelete(): Promise<void> {
  const target = pendingDelete.value
  confirmOpen.value = false
  if (!target) return

  // 先记下位置：删完要选中"下一条"，没有就选上一条
  const index = list.items.value.findIndex((c) => c.id === target.id)
  try {
    await list.remove(target.id)
    push('会话已删除')
    if (selectedId.value === target.id) {
      const next = list.items.value[index] ?? list.items.value[index - 1]
      if (next) select(next.id)
      else selectedId.value = ''
    }
  } catch (e) {
    push(`删除失败：${message(e)}`, 'danger')
  }
}

// ---- 重命名 ----
const renameOpen = ref(false)

async function doRename(title: string): Promise<void> {
  renameOpen.value = false
  if (!selectedId.value) return
  try {
    await list.rename(selectedId.value, title)
    push('已重命名')
  } catch (e) {
    push(`重命名失败：${message(e)}`, 'danger')
  }
}
</script>

<template>
  <div class="flex h-full">
    <AppSidebar
      :items="list.items.value"
      :active-id="selectedId"
      :has-more="list.hasMore.value"
      :loading="list.loading.value"
      :open="sidebarOpen"
      @select="select"
      @delete="askDelete"
      @new-chat="onNewChat"
      @open-settings="settingsPane.openSettings"
      @load-more="list.loadMore"
      @close="sidebarOpen = false"
    />

    <main class="flex min-w-0 flex-1 flex-col bg-canvas">
      <ChatArea
        v-if="settingsPane.pane.value === 'chat'"
        :conversation-id="selectedId"
        :title="currentTitle"
        @rename="renameOpen = true"
        @open-sidebar="sidebarOpen = true"
        @open-settings="settingsPane.openSettings"
      />
      <SettingsView v-else @back="settingsPane.closeSettings" />
    </main>

    <ConfirmDialog
      :open="confirmOpen"
      :title="`删除「${pendingDelete?.title || '新会话'}」？`"
      :message="`这个会话里的 ${pendingDelete?.messageCount ?? 0} 条消息会一起删掉，无法恢复。想留档就先导出 Markdown。`"
      confirm-text="删除会话"
      @confirm="doDelete"
      @cancel="confirmOpen = false"
    />

    <RenameDialog
      :open="renameOpen"
      :initial-title="currentTitle"
      @save="doRename"
      @cancel="renameOpen = false"
    />

    <ToastHost />
  </div>
</template>
