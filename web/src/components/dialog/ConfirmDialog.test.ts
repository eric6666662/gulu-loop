import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ConfirmDialog from './ConfirmDialog.vue'

const props = {
  open: true,
  title: '删除「DeepSeek」？',
  message: '这个服务商下的 3 个模型会一起删掉，模型切换器里也不再有它们。',
  confirmText: '删除服务商',
}

describe('ConfirmDialog', () => {
  it('open=false 时不渲染', () => {
    const w = mount(ConfirmDialog, { props: { ...props, open: false } })

    expect(w.find('[role="dialog"]').exists()).toBe(false)
  })

  it('渲染标题 / 正文 / 危险按钮文案', () => {
    const w = mount(ConfirmDialog, { props })

    expect(w.text()).toContain('删除「DeepSeek」？')
    expect(w.text()).toContain('3 个模型会一起删掉')
    expect(w.find('[data-confirm]').text()).toBe('删除服务商')
  })

  it('确认与取消分别 emit', async () => {
    const w = mount(ConfirmDialog, { props })

    await w.find('[data-confirm]').trigger('click')
    await w.find('[data-cancel]').trigger('click')

    expect(w.emitted('confirm')).toHaveLength(1)
    expect(w.emitted('cancel')).toHaveLength(1)
  })

  it('Esc 关闭（emit cancel）', async () => {
    const w = mount(ConfirmDialog, { props, attachTo: document.body })

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await w.vm.$nextTick()

    expect(w.emitted('cancel')).toHaveLength(1)
    w.unmount()
  })
})
