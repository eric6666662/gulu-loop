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
import { useModelSelection } from './composables/useModelSelection'
import { useSelectedFromQuery } from './composables/useSelectedFromQuery'
import { useSettingsPane } from './composables/useSettingsPane'
import { useToast } from './composables/useToast'

const providers = useProviders()
const settingsPane = useSettingsPane()
const list = useConversationList()
const { selectedId, select } = useSelectedFromQuery(list.items)
const { push } = useToast()

provide('providers', providers)
// 模型选择提到 App 层：ChatArea 在切到设置页时会被 v-if 卸载，
// 放组件内部会在"去设置页激活服务商 → 回聊天页"这条最常见的路径上丢状态，
// 回落也就永远不会发生（ADR-0016 的修正）。
const modelSelection = useModelSelection(providers.activeProvider)
provide('modelSelection', modelSelection)
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

const chatArea = ref<InstanceType<typeof ChatArea> | null>(null)

/**
 * 「新的聊天」是**纯前端**行为（01-需求 #5b）：不发任何请求，
 * 只把右侧切到草稿对话页 + 清空输入框并聚焦。重复点 = 幂等。
 */
function onNewChat(): void {
  select('')
  sidebarOpen.value = false
  chatArea.value?.clearComposer()
  chatArea.value?.focusComposer()
}

/** meta 带回新建会话（草稿态 send 时后端建的）：本地插侧栏 + 选中（标题已在 meta 里） */
function onConversationReady(id: string, title: string): void {
  list.insert({
    id,
    title,
    messageCount: 0,
    lastMessageTime: null,
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
  })
  select(id)
}

/** 一轮结束：改本地列表的消息数与时间（标题已由 meta 带回，不再重拉） */
function onChatDone(id: string, messageCount: number): void {
  list.touch(id, { messageCount, lastMessageTime: new Date().toISOString() })
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
        ref="chatArea"
        :conversation-id="selectedId"
        :title="currentTitle"
        @rename="renameOpen = true"
        @open-sidebar="sidebarOpen = true"
        @open-settings="settingsPane.openSettings"
        @conversation-ready="onConversationReady"
        @done="onChatDone"
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
