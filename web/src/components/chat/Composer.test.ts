import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import Composer from './Composer.vue'

describe('Composer', () => {
  it('空内容按 Enter 不发送', async () => {
    const w = mount(Composer, { props: { streaming: false } })

    await w.find('[data-composer-textarea]').trigger('keydown', { key: 'Enter' })

    expect(w.emitted('send')).toBeUndefined()
  })

  it('Shift+Enter 不发送（换行）', async () => {
    const w = mount(Composer, { props: { streaming: false } })
    await w.find('[data-composer-textarea]').setValue('第一行')

    await w.find('[data-composer-textarea]').trigger('keydown', { key: 'Enter', shiftKey: true })

    expect(w.emitted('send')).toBeUndefined()
  })

  it('有内容按 Enter 发送并清空输入框', async () => {
    const w = mount(Composer, { props: { streaming: false } })
    const ta = w.find('[data-composer-textarea]')
    await ta.setValue('帮我把快排改成泛型')

    await ta.trigger('keydown', { key: 'Enter' })

    expect(w.emitted('send')?.[0]?.[0]).toBe('帮我把快排改成泛型')
    expect((ta.element as HTMLTextAreaElement).value).toBe('')
  })

  it('点主操作按钮发送', async () => {
    const w = mount(Composer, { props: { streaming: false } })
    await w.find('[data-composer-textarea]').setValue('你好')

    await w.find('[data-primary]').trigger('click')

    expect(w.emitted('send')?.[0]?.[0]).toBe('你好')
  })

  it('流式中主操作变成「停止生成」，点击 emit stop', async () => {
    const w = mount(Composer, { props: { streaming: true } })

    const btn = w.find('[data-primary]')
    expect(btn.attributes('aria-label')).toBe('停止生成')
    await btn.trigger('click')

    expect(w.emitted('stop')).toHaveLength(1)
  })

  it('空输入时发送按钮禁用', () => {
    const w = mount(Composer, { props: { streaming: false } })

    expect(w.find('[data-primary]').attributes('disabled')).toBeDefined()
  })

  it('disabled 时输入框与按钮都禁用', () => {
    const w = mount(Composer, { props: { streaming: false, disabled: true } })

    expect(w.find('[data-composer-textarea]').attributes('disabled')).toBeDefined()
    expect(w.find('[data-primary]').attributes('disabled')).toBeDefined()
  })

  it('暴露 focus / clear 给父组件（草稿态重复点新建时清空并聚焦）', async () => {
    const w = mount(Composer, { props: { streaming: false }, attachTo: document.body })
    await w.find('[data-composer-textarea]').setValue('半截草稿')

    ;(w.vm as unknown as { clear: () => void }).clear()
    await w.vm.$nextTick()

    expect((w.find('[data-composer-textarea]').element as HTMLTextAreaElement).value).toBe('')
    w.unmount()
  })
})

describe('Composer · 模型按钮', () => {
  const models = [
    { id: 'deepseek-v4-pro', name: 'V4 Pro', serviceProvider: 'deepseek' },
    { id: 'kimi-k2', name: 'kimi-k2', serviceProvider: 'moonshot' },
  ]

  it('图标栏左侧显示当前模型；选中其它模型 emit update:model', async () => {
    const w = mount(Composer, { props: { streaming: false, models, model: 'deepseek-v4-pro' } })

    expect(w.find('[data-model-name]').text()).toBe('deepseek-v4-pro')

    await w.find('[data-model-switch]').trigger('click')
    await w.findAll('[data-model-option]')[1].trigger('click')

    expect(w.emitted('update:model')?.[0]?.[0]).toBe('kimi-k2')
  })

  it('没有可选模型时模型按钮禁用', () => {
    const w = mount(Composer, { props: { streaming: false, models: [], model: '' } })

    expect(w.find('[data-model-switch]').attributes('disabled')).toBeDefined()
  })
})
