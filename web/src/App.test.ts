import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import App from './App.vue'
import * as conversationApi from './api/conversations'
import * as providerApi from './api/providers'

describe('App 启动加载', () => {
  afterEach(() => vi.restoreAllMocks())

  it('挂载时同时加载会话列表与服务商列表', async () => {
    const conversations = vi
      .spyOn(conversationApi, 'list')
      .mockResolvedValue({ list: [], page: 1, pageSize: 10, total: 0 })
    const providers = vi.spyOn(providerApi, 'list').mockResolvedValue([])

    mount(App)

    await vi.waitFor(() => expect(conversations).toHaveBeenCalled())
    // 服务商列表是设置页与模型切换器的数据源，必须和会话列表一起加载
    await vi.waitFor(() => expect(providers).toHaveBeenCalled())
  })
})

describe('App · 草稿态（01-需求 #5b）', () => {
  afterEach(() => vi.restoreAllMocks())

  it('点「新的聊天」是纯前端：不创建会话，右侧切到草稿对话页', async () => {
    vi.spyOn(conversationApi, 'list').mockResolvedValue({ list: [], page: 1, pageSize: 10, total: 0 })
    vi.spyOn(providerApi, 'list').mockResolvedValue([])
    const create = vi.spyOn(conversationApi, 'create')

    const w = mount(App)
    await vi.waitFor(() => expect(conversationApi.list).toHaveBeenCalled())

    await w.find('[data-new-chat]').trigger('click')
    await vi.waitFor(() => expect(w.text()).toContain('开始新的对话'))

    expect(create).not.toHaveBeenCalled()
  })
})
