import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { useSelectedFromQuery } from './useSelectedFromQuery'
import type { QueryIO } from './useSelectedFromQuery'

function fakeIO(initial: string | null) {
  const written: (string | null)[] = []
  const io: QueryIO = {
    read: () => initial,
    write: (id) => written.push(id),
  }
  return { io, written }
}

describe('useSelectedFromQuery', () => {
  it('无 ?c= 时选第一条，且不写地址', () => {
    const items = ref([{ id: 'a' }, { id: 'b' }])
    const { io, written } = fakeIO(null)

    const { selectedId } = useSelectedFromQuery(items, io)

    expect(selectedId.value).toBe('a')
    expect(written).toEqual([])
  })

  it('有效 ?c= 时选它', () => {
    const items = ref([{ id: 'a' }, { id: 'b' }])
    const { io } = fakeIO('b')

    const { selectedId } = useSelectedFromQuery(items, io)

    expect(selectedId.value).toBe('b')
  })

  it('无效 ?c= 时回落第一条并清掉参数', () => {
    const items = ref([{ id: 'a' }])
    const { io, written } = fakeIO('ghost')

    const { selectedId } = useSelectedFromQuery(items, io)

    expect(selectedId.value).toBe('a')
    expect(written.at(-1)).toBeNull() // 清参
  })

  it('列表后到时 ?c= 仍生效', async () => {
    const items = ref<{ id: string }[]>([])
    const { io } = fakeIO('b')

    const { selectedId } = useSelectedFromQuery(items, io)
    items.value = [{ id: 'a' }, { id: 'b' }]
    await Promise.resolve()

    expect(selectedId.value).toBe('b')
  })

  it('select 后回写地址，且只用 replaceState（不新增历史）', () => {
    const items = ref([{ id: 'a' }, { id: 'b' }])
    const { io, written } = fakeIO(null)
    const replaceSpy = vi.spyOn(window.history, 'replaceState')
    const pushSpy = vi.spyOn(window.history, 'pushState')

    const { selectedId, select } = useSelectedFromQuery(items, io)
    select('b')

    expect(selectedId.value).toBe('b')
    expect(written.at(-1)).toBe('b')
    expect(pushSpy).not.toHaveBeenCalled()
    replaceSpy.mockRestore()
    pushSpy.mockRestore()
  })
})
