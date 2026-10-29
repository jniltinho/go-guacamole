import { computed, ref, watch } from 'vue'

import { DEFAULT_DARK, DEFAULT_LIGHT, getTheme, STORAGE_KEY, THEMES } from '../themes'

function systemDefault() {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? DEFAULT_DARK : DEFAULT_LIGHT
}

function stored(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

const current = ref<string>(getTheme(stored())?.id ?? systemDefault())
const inClient = ref(false)

function applyToDocument() {
  const t = getTheme(current.value) ?? getTheme(DEFAULT_LIGHT)!
  const root = document.documentElement
  root.dataset.theme = t.id
  root.style.colorScheme = t.scheme
  root.classList.toggle('has-clock', !!t.clock)
  root.classList.toggle('in-client', inClient.value)
  document.querySelectorAll<HTMLLinkElement>('link[rel~="icon"]').forEach((l) => {
    l.type = 'image/svg+xml'
    l.href = t.favicon
  })
}

watch([current, inClient], applyToDocument, { immediate: true })

/** Global theme state (one per app). */
export function useTheme() {
  const theme = computed(() => getTheme(current.value)!)
  function setTheme(id: string) {
    if (!getTheme(id)) return
    current.value = id
    try {
      localStorage.setItem(STORAGE_KEY, id)
    } catch {
      /* storage unavailable */
    }
  }
  return {
    themes: THEMES,
    theme,
    themeId: computed(() => current.value),
    setTheme,
    /** Disables decorations/animations while a remote session is shown. */
    setInClient: (v: boolean) => (inClient.value = v),
    inClient: computed(() => inClient.value),
  }
}
