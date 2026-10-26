import { ref, type Ref } from 'vue'
import type { Provider } from '../api/providers'

export type Pane = 'chat' | 'settings'

export type Step = 'list' | 'edit'

export interface UseSettingsPane {
  pane: Ref<Pane>
  step: Ref<Step>
  editing: Ref<Provider | null>
  openSettings: () => void
  closeSettings: () => void
  editProvider: (p: Provider) => void
  createProvider: () => void
  backToList: () => void
}

/**
 * 右侧主区的视图状态机（ADR-0012）：聊天 / 服务商设置互斥，
 * 设置内部再分列表 / 编辑两步（02-前端设计.md §2.1）。
 *
 * 只切视图、不碰数据：谁去拉列表、谁去保存，都是页面的事。
 */
export function useSettingsPane(): UseSettingsPane {
  const pane = ref<Pane>('chat')
  const step = ref<Step>('list')
  const editing = ref<Provider | null>(null)

  /** 进设置并落在列表页（每次进来都从列表开始，避免停在半填的表单上） */
  function openSettings(): void {
    pane.value = 'settings'
    step.value = 'list'
    editing.value = null
  }

  /** 回聊天；顺手复位步与编辑对象，下次进来是干净的列表 */
  function closeSettings(): void {
    pane.value = 'chat'
    step.value = 'list'
    editing.value = null
  }

  function editProvider(p: Provider): void {
    pane.value = 'settings'
    step.value = 'edit'
    editing.value = p
  }

  function createProvider(): void {
    pane.value = 'settings'
    step.value = 'edit'
    editing.value = null
  }

  /** 回列表：未保存的改动直接丢弃（02-ui设计.md 待确认点 10） */
  function backToList(): void {
    step.value = 'list'
    editing.value = null
  }

  return { pane, step, editing, openSettings, closeSettings, editProvider, createProvider, backToList }
}
