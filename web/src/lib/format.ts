const WEEKDAYS = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']

function pad(n: number): string {
  return String(n).padStart(2, '0')
}

/** 相对时间（docs/设计稿.md §8 文案口径）：今天 HH:MM / 昨天 / 周X / MM-DD */
export function relativeTime(iso: string | null | undefined, now: Date = new Date()): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''

  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const startOfDay = new Date(d.getFullYear(), d.getMonth(), d.getDate())
  const diffDays = Math.round((startOfToday.getTime() - startOfDay.getTime()) / 86_400_000)

  if (diffDays <= 0) return `${pad(d.getHours())}:${pad(d.getMinutes())}`
  if (diffDays === 1) return '昨天'
  if (diffDays < 7) return WEEKDAYS[d.getDay()]
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** 「N 条消息」 */
export function messageCount(n: number): string {
  return `${n} 条消息`
}
