import { ref, type Ref } from 'vue'

export type ToastTone = 'ok' | 'danger'
export interface Toast {
  id: number
  text: string
  tone: ToastTone
}

const toasts = ref<Toast[]>([])
let seq = 0

/** 弹一条 toast；2.2s 自动消失（docs/设计稿.md §5 Toast） */
export function pushToast(text: string, tone: ToastTone = 'ok'): void {
  const id = ++seq
  toasts.value = [...toasts.value, { id, text, tone }]
  setTimeout(() => {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }, 2200)
}

export function useToast(): { toasts: Ref<Toast[]>; push: typeof pushToast } {
  return { toasts, push: pushToast }
}
