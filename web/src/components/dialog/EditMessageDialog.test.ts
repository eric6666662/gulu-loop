import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import EditMessageDialog from './EditMessageDialog.vue'

describe('EditMessageDialog', () => {
  it('open=false 不渲染', () => {
    const w = mount(EditMessageDialog, { props: { open: false, initialContent: '旧内容' } })
    expect(w.find('[role="dialog"]').exists()).toBe(false)
  })

  it('预填原消息内容', () => {
    const w = mount(EditMessageDialog, { props: { open: true, initialContent: '旧内容' } })
    expect((w.find('[data-content-input]').element as HTMLTextAreaElement).value).toBe('旧内容')
  })

  it('空内容拦下并给文案', async () => {
    const w = mount(EditMessageDialog, { props: { open: true, initialContent: '旧内容' } })

    await w.find('[data-content-input]').setValue('   ')
    await w.find('[data-save]').trigger('click')

    expect(w.emitted('save')).toBeUndefined()
    expect(w.text()).toContain('消息不能为空')
  })

  it('合法时 emit save（去两端空白）', async () => {
    const w = mount(EditMessageDialog, { props: { open: true, initialContent: '旧内容' } })

    await w.find('[data-content-input]').setValue('  改过的内容  ')
    await w.find('[data-save]').trigger('click')

    expect(w.emitted('save')?.[0]?.[0]).toBe('改过的内容')
  })

  it('取消 emit cancel', async () => {
    const w = mount(EditMessageDialog, { props: { open: true, initialContent: '旧内容' } })
    await w.find('[data-cancel]').trigger('click')
    expect(w.emitted('cancel')).toHaveLength(1)
  })
})
