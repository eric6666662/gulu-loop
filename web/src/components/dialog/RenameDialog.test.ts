import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import RenameDialog from './RenameDialog.vue'

describe('RenameDialog', () => {
  it('open=false 不渲染', () => {
    const w = mount(RenameDialog, { props: { open: false, initialTitle: '旧标题' } })
    expect(w.find('[role="dialog"]').exists()).toBe(false)
  })

  it('预填当前标题', () => {
    const w = mount(RenameDialog, { props: { open: true, initialTitle: '旧标题' } })
    expect((w.find('[data-title-input]').element as HTMLInputElement).value).toBe('旧标题')
  })

  it('空标题拦下并给文案', async () => {
    const w = mount(RenameDialog, { props: { open: true, initialTitle: '旧标题' } })

    await w.find('[data-title-input]').setValue('   ')
    await w.find('[data-save]').trigger('click')

    expect(w.emitted('save')).toBeUndefined()
    expect(w.text()).toContain('标题不能为空')
  })

  it('合法时 emit save（去两端空白）', async () => {
    const w = mount(RenameDialog, { props: { open: true, initialTitle: '旧标题' } })

    await w.find('[data-title-input]').setValue('  新标题  ')
    await w.find('[data-save]').trigger('click')

    expect(w.emitted('save')?.[0]?.[0]).toBe('新标题')
  })

  it('取消 emit cancel', async () => {
    const w = mount(RenameDialog, { props: { open: true, initialTitle: '旧标题' } })

    await w.find('[data-cancel]').trigger('click')

    expect(w.emitted('cancel')).toHaveLength(1)
  })
})
