import { computed, ref, watch } from 'vue'

import { resolveEffects } from '../lib/effects'

// Applies the effects preference on html[data-effects]; cached for first paint.
export const EFFECTS_KEY = 'gg-effects'

function cached(): string {
  try {
    return localStorage.getItem(EFFECTS_KEY) ?? ''
  } catch {
    return ''
  }
}

const media =
  typeof window !== 'undefined' ? window.matchMedia?.('(prefers-reduced-motion: reduce)') : undefined
const pref = ref(cached())
const osReduced = ref(!!media?.matches)
media?.addEventListener?.('change', (e) => (osReduced.value = e.matches))

const effective = computed(() => resolveEffects(pref.value, osReduced.value))
watch(effective, (v) => (document.documentElement.dataset.effects = v), { immediate: true })

export function useEffects() {
  function setEffects(v: string) {
    pref.value = v
    try {
      if (v) localStorage.setItem(EFFECTS_KEY, v)
      else localStorage.removeItem(EFFECTS_KEY)
    } catch {
      /* storage unavailable */
    }
  }
  return { preference: computed(() => pref.value), effective, setEffects }
}
