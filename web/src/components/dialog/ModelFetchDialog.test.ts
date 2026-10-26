import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ModelFetchDialog from './ModelFetchDialog.vue'
import * as api from '../../api/providers'
import { ApiError } from '../../api/client'

const baseProps = {
  open: true,
  providerName: 'DeepSeek',
  serviceProvider: 'deepseek',
  baseUrl: 'https://api.deepseek.com/v1',
  apiKey: '',
  existingIds: ['deepseek-v4-pro', 'deepseek-v4-flash'],
}

async function mountWith(ids: string[]) {
  vi.spyOn(api, 'fetchModels').mockResolvedValue(ids)
  const w = mount(ModelFetchDialog, { props: baseProps, attachTo: document.body })
  await flushPromises()
  return w
}

describe('ModelFetchDialog', () => {
  afterEach(() => vi.restoreAllMocks())

  it('打开时拉取上游模型，已在清单里的置灰并标「已添加」', async () => {
    const w = await mountWith(['deepseek-v4-pro', 'deepseek-v4-flash', 'deepseek-chat'])

    const rows = w.findAll('[data-fetch-item]')
    expect(rows).toHaveLength(3)
    expect(w.findAll('[data-added]')).toHaveLength(2)
    expect((rows[0].element as HTMLInputElement).disabled).toBe(true)
    expect((rows[2].element as HTMLInputElement).disabled).toBe(false)

    w.unmount()
  })

  it('只 emit 新增的 id（不重复添加已有的）', async () => {
    const w = await mountWith(['deepseek-v4-pro', 'deepseek-chat', 'deepseek-coder'])

    const checkboxes = w.findAll('[data-fetch-item]')
    await checkboxes[1].setValue(true) // deepseek-chat
    await checkboxes[2].setValue(true) // deepseek-coder
    await w.find('[data-confirm]').trigger('click')

    expect(w.emitted('confirm')?.[0]?.[0]).toEqual(['deepseek-chat', 'deepseek-coder'])

    w.unmount()
  })

  it('502 时展示上游原因，且弹层不关闭', async () => {
    vi.spyOn(api, 'fetchModels').mockRejectedValue(new ApiError(502, '上游返回 401：api_key 无效'))
    const w = mount(ModelFetchDialog, { props: baseProps, attachTo: document.body })
    await flushPromises()

    expect(w.find('[data-fetch-error]').text()).toContain('上游返回 401：api_key 无效')
    expect(w.emitted('close')).toBeUndefined()

    w.unmount()
  })

  it('取消 emit close', async () => {
    const w = await mountWith(['deepseek-chat'])

    await w.find('[data-cancel]').trigger('click')

    expect(w.emitted('close')).toHaveLength(1)
    w.unmount()
  })

  it('open=false 时不渲染', async () => {
    vi.spyOn(api, 'fetchModels').mockResolvedValue([])
    const w = mount(ModelFetchDialog, { props: { ...baseProps, open: false } })
    await flushPromises()

    expect(w.find('[role="dialog"]').exists()).toBe(false)
  })
})
