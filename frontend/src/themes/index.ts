// Theme registry: every ./<id>/index.ts is loaded (with its theme.css) at startup.
import './base.css'
import type { ThemeMeta } from './define'

const modules = import.meta.glob<{ default: ThemeMeta }>('./*/index.ts', { eager: true })

export const THEMES: ThemeMeta[] = Object.values(modules)
  .map((m) => m.default)
  .sort((a, b) => a.order - b.order || a.id.localeCompare(b.id))

export const THEME_IDS = THEMES.map((t) => t.id)
export const DEFAULT_LIGHT = 'light'
export const DEFAULT_DARK = 'dark'
export const STORAGE_KEY = 'gg-theme'

export function getTheme(id: string | null | undefined): ThemeMeta | undefined {
  return THEMES.find((t) => t.id === id)
}

export type { ThemeMeta }
