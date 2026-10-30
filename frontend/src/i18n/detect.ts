export const LOCALES = ['en', 'pt-BR'] as const
export type Locale = (typeof LOCALES)[number]

/** Saved preference → browser language (pt* → pt-BR) → English. */
export function detectLocale(saved: string | null | undefined, browser: readonly string[] = []): Locale {
  if (saved && (LOCALES as readonly string[]).includes(saved)) return saved as Locale
  for (const l of browser) {
    const v = l.toLowerCase()
    if (v.startsWith('pt')) return 'pt-BR'
    if (v.startsWith('en')) return 'en'
  }
  return 'en'
}
