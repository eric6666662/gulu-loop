import { describe, expect, it } from 'vitest'
import { messageCount, relativeTime } from './format'

// 固定"现在"：2026-10-26（周日）15:00 本地时间
const now = new Date(2026, 9, 26, 15, 0, 0)

describe('lib/format', () => {
  it('今天显示 HH:MM', () => {
    expect(relativeTime(new Date(2026, 9, 26, 10, 31).toISOString(), now)).toBe('10:31')
  })

  it('昨天显示「昨天」', () => {
    expect(relativeTime(new Date(2026, 9, 25, 22, 0).toISOString(), now)).toBe('昨天')
  })

  it('本周内显示周几', () => {
    expect(relativeTime(new Date(2026, 9, 21, 9, 0).toISOString(), now)).toBe('周三')
  })

  it('更早显示 MM-DD（跨月也对）', () => {
    expect(relativeTime(new Date(2026, 8, 18, 9, 0).toISOString(), now)).toBe('09-18')
  })

  it('空值不崩', () => {
    expect(relativeTime(null, now)).toBe('')
  })

  it('消息数文案', () => {
    expect(messageCount(18)).toBe('18 条消息')
    expect(messageCount(0)).toBe('0 条消息')
  })
})
