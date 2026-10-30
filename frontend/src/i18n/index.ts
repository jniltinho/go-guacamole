import { createI18n } from 'vue-i18n'

import en from '../locales/en.json'
import ptBR from '../locales/pt-BR.json'

import { detectLocale } from './detect'

export { LOCALES, detectLocale, type Locale } from './detect'
const STORAGE_KEY = 'gg-locale'

function storedLocale(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(storedLocale()),
  fallbackLocale: 'en',
  messages: { en, 'pt-BR': ptBR },
  datetimeFormats: {
    en: {
      short: { year: 'numeric', month: 'short', day: 'numeric' },
      long: { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' },
      time: { hour: '2-digit', minute: '2-digit' },
    },
    'pt-BR': {
      short: { year: 'numeric', month: 'short', day: 'numeric' },
      long: { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' },
      time: { hour: '2-digit', minute: '2-digit' },
    },
  },
})

export function setLocale(l: string) {
  const loc = detectLocale(l)
  i18n.global.locale.value = loc
  document.documentElement.lang = loc
  try {
    localStorage.setItem(STORAGE_KEY, loc)
  } catch {
    /* storage unavailable */
  }
}
