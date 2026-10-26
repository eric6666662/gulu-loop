import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelRow from './ModelRow.vue'

describe('ModelRow', () => {
  it('渲染模型标识与展示名', () => {
    const w = mount(ModelRow, {
      props: { model: { id: 'deepseek-v4-pro', name: 'V4 Pro' }, isDefault: true },
    })

    expect((w.find('[data-model-id]').element as HTMLInputElement).value).toBe('deepseek-v4-pro')
    expect((w.find('[data-model-name]').element as HTMLInputElement).value).toBe('V4 Pro')
  })

  it('改标识时 emit update，带上完整模型对象', async () => {
    const w = mount(ModelRow, {
      props: { model: { id: 'a', name: 'A' }, isDefault: false },
    })

    const input = w.find('[data-model-id]')
    await input.setValue('b')

    expect(w.emitted('update')?.at(-1)?.[0]).toEqual({ id: 'b', name: 'A' })
  })

  it('改展示名时 emit update', async () => {
    const w = mount(ModelRow, {
      props: { model: { id: 'a', name: 'A' }, isDefault: false },
    })

    await w.find('[data-model-name]').setValue('一号')

    expect(w.emitted('update')?.at(-1)?.[0]).toEqual({ id: 'a', name: '一号' })
  })

  it('点单选 emit set-default', async () => {
    const w = mount(ModelRow, {
      props: { model: { id: 'a', name: 'A' }, isDefault: false },
    })

    await w.find('[data-set-default]').trigger('change')

    expect(w.emitted('set-default')).toHaveLength(1)
  })

  it('默认行没有删除按钮（避免删掉默认模型）', () => {
    const w = mount(ModelRow, {
      props: { model: { id: 'a', name: 'A' }, isDefault: true },
    })

    expect(w.find('[data-model-remove]').exists()).toBe(false)
  })

  it('非默认行点删除 emit remove', async () => {
    const w = mount(ModelRow, {
      props: { model: { id: 'a', name: 'A' }, isDefault: false },
    })

    await w.find('[data-model-remove]').trigger('click')

    expect(w.emitted('remove')).toHaveLength(1)
  })
})
