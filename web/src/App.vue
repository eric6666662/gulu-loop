<script setup lang="ts">
import { onMounted, provide } from 'vue'
import AppSidebar from './components/layout/AppSidebar.vue'
import ChatArea from './components/chat/ChatArea.vue'
import SettingsView from './components/settings/SettingsView.vue'
import ToastHost from './components/ui/ToastHost.vue'
import { useProviders } from './composables/useProviders'
import { useSettingsPane } from './composables/useSettingsPane'

const providers = useProviders()
const settingsPane = useSettingsPane()

provide('providers', providers)
provide('settingsPane', settingsPane)
onMounted(providers.load)
</script>

<template>
  <div class="flex h-full">
    <AppSidebar @open-settings="settingsPane.openSettings" />
    <main class="flex min-w-0 flex-1 flex-col bg-canvas">
      <ChatArea v-if="settingsPane.pane.value === 'chat'" />
      <SettingsView v-else @back="settingsPane.closeSettings" />
    </main>
    <ToastHost />
  </div>
</template>
