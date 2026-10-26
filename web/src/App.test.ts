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
