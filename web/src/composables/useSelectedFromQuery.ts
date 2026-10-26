import { ref, watch, type Ref } from 'vue'

export interface QueryIO {
  read: () => string | null
  write: (id: string | null) => void
}

export interface UseSelectedFromQuery {
  selectedId: Ref<string>
  select: (id: string) => void
}

/** 默认读写地址栏：?c=<id>（不是路由，只是查询参数，02-前端设计.md §4） */
const defaultIO: QueryIO = {
  read: () => new URLSearchParams(window.location.search).get('c'),
  write: (id) => {
    const url = new URL(window.location.href)
    if (id) url.searchParams.set('c', id)
    else url.searchParams.delete('c')
    // 必须 replaceState：pushState 会让后退键变成"退回上一个会话"
    window.history.replaceState({}, '', url)
  },
}

/**
 * 当前会话的深链（02-前端设计.md §4）：
 *   - 无 ?c= → 选第一条，**不**写地址（保持地址干净）
 *   - 有效 ?c= → 选它（列表后到时也生效）
 *   - 无效 ?c= → 回落第一条并清掉参数（不留死链）
 *   - 用户主动切换（select）→ 回写 ?c=
 */
export function useSelectedFromQuery(
  items: Ref<{ id: string }[]>,
  io: QueryIO = defaultIO,
): UseSelectedFromQuery {
  const selectedId = ref('')
  const wanted = io.read()

  watch(
    items,
    (list) => {
      if (list.length === 0) return
      // 已经选好了且仍有效 → 不动
      if (selectedId.value && list.some((i) => i.id === selectedId.value)) return
      // 地址栏指定的会话到了 → 用它
      if (wanted && list.some((i) => i.id === wanted)) {
        selectedId.value = wanted
        return
      }
      // 地址栏指定了但不存在 → 回落第一条 + 清参
      selectedId.value = list[0].id
      if (wanted) io.write(null)
    },
    { immediate: true },
  )

  function select(id: string): void {
    selectedId.value = id
    io.write(id)
  }

  return { selectedId, select }
}
