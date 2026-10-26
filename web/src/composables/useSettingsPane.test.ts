import { describe, expect, it } from 'vitest'
import { useSettingsPane } from './useSettingsPane'
import type { Provider } from '../api/providers'

const provider: Provider = {
  serviceProvider: 'deepseek',
  website: 'https://platform.deepseek.com',
  baseUrl: 'https://api.deepseek.com/v1',
  model: 'deepseek-v4-pro',
  models: [{ id: 'deepseek-v4-pro', name: 'V4 Pro' }],
  hasApiKey: true,
  createdAt: '2026-10-26T00:00:00Z',
  updatedAt: '2026-10-26T00:00:00Z',
}

describe('useSettingsPane', () => {
  it('初始：聊天视图、列表步、无编辑对象', () => {
    const { pane, step, editing } = useSettingsPane()

    expect(pane.value).toBe('chat')
    expect(step.value).toBe('list')
    expect(editing.value).toBeNull()
  })

  it('openSettings 进设置并落在列表页', () => {
    const { pane, step, openSettings } = useSettingsPane()

    openSettings()

    expect(pane.value).toBe('settings')
    expect(step.value).toBe('list')
  })

  it('editProvider 进编辑页并带上该服务商', () => {
    const { step, editing, openSettings, editProvider } = useSettingsPane()

    openSettings()
    editProvider(provider)

    expect(step.value).toBe('edit')
    expect(editing.value).toEqual(provider)
  })

  it('createProvider 进编辑页且 editing 为 null（新增态）', () => {
    const { step, editing, editProvider, createProvider } = useSettingsPane()

    editProvider(provider)
    createProvider()

    expect(step.value).toBe('edit')
    expect(editing.value).toBeNull()
  })

  it('backToList 回列表并清掉编辑对象（丢弃未保存改动）', () => {
    const { step, editing, createProvider, backToList } = useSettingsPane()

    createProvider()
    backToList()

    expect(step.value).toBe('list')
    expect(editing.value).toBeNull()
  })

  it('closeSettings 回聊天，并复位到列表步', () => {
    const { pane, step, editing, openSettings, editProvider, closeSettings } = useSettingsPane()

    openSettings()
    editProvider(provider)
    closeSettings()

    expect(pane.value).toBe('chat')
    expect(step.value).toBe('list')
    expect(editing.value).toBeNull()
  })
})
