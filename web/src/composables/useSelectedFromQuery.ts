import { ref, watch, type Ref } from 'vue'

export interface QueryIO {
  read: () => string | null
  write: (id: string | null) => void
}

export interface UseSelectedFromQuery {
  selectedId: Ref<string>
  /** 草稿态：用户刚点了「新的聊天」，还没有会话（01-需求 #5b） */
  draft: Ref<boolean>
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
 *   - `select('')` → 草稿态：清掉 ?c=，且不被自动回落覆盖（01-需求 #5b）
 */
export function useSelectedFromQuery(
  items: Ref<{ id: string }[]>,
  io: QueryIO = defaultIO,
): UseSelectedFromQuery {
  const selectedId = ref('')
  const draft = ref(false)
  const wanted = io.read()

  watch(
    items,
    (list) => {
      // 草稿态：用户主动点了「新的聊天」，不要自动回落到第一条
      if (draft.value) return
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

  /** 选中会话；传空串表示草稿态。只有这里回写地址栏。 */
  function select(id: string): void {
    if (!id) {
      draft.value = true
      selectedId.value = ''
      io.write(null)
      return
    }
    draft.value = false
    selectedId.value = id
    io.write(id)
  }

  return { selectedId, draft, select }
}
