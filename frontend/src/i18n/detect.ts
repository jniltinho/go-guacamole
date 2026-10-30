export const LOCALES = ['en', 'pt-BR'] as const
export type Locale = (typeof LOCALES)[number]

/** Saved preference, else English (the browser language is not used). */
export function detectLocale(saved: string | null | undefined): Locale {
  if (saved && (LOCALES as readonly string[]).includes(saved)) return saved as Locale
  return 'en'
}
