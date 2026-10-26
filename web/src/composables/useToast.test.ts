import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { pushToast, useToast } from './useToast'

describe('useToast', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('push 后出现，2.2s 后自动消失', () => {
    const { toasts } = useToast()
    const before = toasts.value.length

    pushToast('服务商已保存')

    expect(toasts.value).toHaveLength(before + 1)
    expect(toasts.value.at(-1)?.text).toBe('服务商已保存')

    vi.advanceTimersByTime(2300)
    expect(toasts.value).toHaveLength(before)
  })

  it('danger 语气能透传（连通失败用）', () => {
    const { toasts } = useToast()

    pushToast('DeepSeek 连不上：401 api_key 无效', 'danger')

    expect(toasts.value.at(-1)?.tone).toBe('danger')
  })
})
