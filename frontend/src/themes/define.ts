// Theme definition helpers. A theme is a folder <id>/ with:
//   theme.css  — CSS custom properties under html[data-theme='<id>'] (+ optional decorations)
//   index.ts   — `export default defineTheme({...})` metadata
// The registry (./index.ts) discovers every folder automatically.

export interface ThemeMeta {
  /** Unique id, equal to the folder name and the data-theme attribute value. */
  id: string
  /** English label (fallback when no `themes.<id>` translation exists). */
  label: string
  /** Base color scheme, used for form controls and scrollbars. */
  scheme: 'light' | 'dark'
  /** Position in the theme picker. */
  order: number
  /** Show the retro taskbar clock in the sidebar footer. */
  clock?: boolean
  /** Renders a decorative background animation (e.g. Matrix rain). */
  animated?: boolean
  /** Favicon data URL. */
  favicon: string
}

export function defineTheme(meta: ThemeMeta): ThemeMeta {
  return meta
}

export function svgIcon(markup: string): string {
  return 'data:image/svg+xml,' + encodeURIComponent(markup)
}

/** Terminal-prompt favicon: a chevron and an underscore on a square. */
export function promptIcon(bg: string, fg: string, fg2 = fg): string {
  return svgIcon(
    `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="${bg}"/>` +
      `<path d="M8 10l6 6-6 6" stroke="${fg}" stroke-width="3" fill="none"/><path d="M16 23h8" stroke="${fg2}" stroke-width="3"/></svg>`,
  )
}
