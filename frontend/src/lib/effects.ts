// Visual effects preference: full | reduced | off; "" follows the OS reduced-motion setting.
export type Effects = 'full' | 'reduced' | 'off'

export function resolveEffects(pref: string | null | undefined, reducedMotion: boolean): Effects {
  if (pref === 'full' || pref === 'reduced' || pref === 'off') return pref
  return reducedMotion ? 'reduced' : 'full'
}
