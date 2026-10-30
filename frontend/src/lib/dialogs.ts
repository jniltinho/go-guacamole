import { reactive } from 'vue'

// Themed replacements for confirm(), alert() and prompt() (as in AD Manager): every helper returns a Promise and
// AppDialogs.vue (one per app) renders the open one with Modal.vue.
export interface DialogOpts {
  title?: string
  ok?: string
  danger?: boolean
}
type Open =
  | { kind: 'confirm'; msg: string; opts: DialogOpts; resolve: (v: boolean) => void }
  | { kind: 'alert'; msg: string; opts: DialogOpts; resolve: (v: void) => void }
  | {
      kind: 'text'
      msg: string
      opts: DialogOpts & { label?: string; maxlength?: number }
      resolve: (v: string | null) => void
    }
  | { kind: 'typed'; msg: string; expected: string; opts: DialogOpts; resolve: (v: boolean) => void }

export const dialogState = reactive<{ open: Open | null }>({ open: null })

type Request = Open extends infer O ? (O extends unknown ? Omit<O, 'resolve'> : never) : never

// a new dialog answers the one still open as cancelled
function show<T>(o: Request): Promise<T> {
  cancelDialogs()
  return new Promise<T>((resolve) => {
    dialogState.open = { ...o, resolve } as unknown as Open
  })
}

export const askConfirm = (msg: string, opts: DialogOpts = {}) =>
  show<boolean>({ kind: 'confirm', msg, opts })
export const showAlert = (msg: string, opts: DialogOpts = {}) => show<void>({ kind: 'alert', msg, opts })
// askText resolves to the trimmed text, or null when cancelled (an empty answer cannot be submitted)
export const askText = (msg: string, opts: DialogOpts & { label?: string; maxlength?: number } = {}) =>
  show<string | null>({ kind: 'text', msg, opts })
// confirmTyped guards destructive actions: OK only when the expected text is typed exactly
export const confirmTyped = (msg: string, expected: string, opts: DialogOpts = {}) =>
  show<boolean>({ kind: 'typed', msg, expected, opts })

export function answerDialog(v: boolean | string | null | void) {
  const o = dialogState.open
  dialogState.open = null
  ;(o?.resolve as ((x: unknown) => void) | undefined)?.(v)
}

// cancelDialogs answers the open dialog as cancelled (navigation, sign-out)
export function cancelDialogs() {
  const o = dialogState.open
  if (!o) return
  answerDialog(o.kind === 'text' ? null : o.kind === 'alert' ? undefined : false)
}
