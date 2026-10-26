import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EmptyState from './EmptyState.vue'

describe('EmptyState', () => {
  it('草稿态（还没会话）用「开始新的对话」', () => {
    const w = mount(EmptyState, { props: { draft: true } })

    expect(w.text()).toContain('开始新的对话')
    expect(w.text()).not.toContain('这个会话还是空的')
  })

  it('已存在的空会话用「这个会话还是空的」', () => {
    const w = mount(EmptyState, { props: { draft: false } })

    expect(w.text()).toContain('这个会话还是空的')
  })
})
