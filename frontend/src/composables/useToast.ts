import { ref } from 'vue'

// Transient results (saved, copied, failed) announced in a live region.
export interface Toast {
  id: number
  text: string
  kind: 'success' | 'error' | 'info'
}

const toasts = ref<Toast[]>([])
let seq = 0

export function useToast() {
  function dismiss(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }
  function show(text: string, kind: Toast['kind'] = 'success', ms = 4000) {
    const id = ++seq
    toasts.value = [...toasts.value, { id, text, kind }].slice(-4)
    if (ms > 0) setTimeout(() => dismiss(id), ms)
  }
  return { toasts, show, dismiss }
}
