// Token resolution and WCAG contrast for theme tests (openspec ui-theming).
// Parses the token block of a theme.css plus the shared defaults of base.css.

export type Tokens = Record<string, string>
export type RGBA = [number, number, number, number]

/** Custom properties of the first rule whose selector list contains `selector`. */
export function parseTokens(css: string, selector: string): Tokens {
  const out: Tokens = {}
  const re = /([^{}]+)\{([^{}]*)\}/g
  for (let m = re.exec(css); m; m = re.exec(css)) {
    const sels = m[1]!
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .split(',')
      .map((s) => s.trim())
    if (!sels.includes(selector)) continue
    for (const decl of m[2]!.replace(/\/\*[\s\S]*?\*\//g, '').split(';')) {
      const i = decl.indexOf(':')
      const name = decl.slice(0, i).trim()
      if (i > 0 && name.startsWith('--')) out[name] = decl.slice(i + 1).trim()
    }
    break
  }
  return out
}

/** Resolves var(--x[, fallback]) references (cycles and unknowns resolve to ''). */
export function resolve(tokens: Tokens, name: string, seen = new Set<string>()): string {
  const v = tokens[name]
  if (v === undefined || seen.has(name)) return ''
  seen.add(name)
  return v.replace(/var\((--[\w-]+)\s*(?:,\s*([^)]*))?\)/g, (_, ref: string, fb?: string) => {
    const r = resolve(tokens, ref, new Set(seen))
    return r || (fb ?? '').trim()
  })
}

export function parseColor(v: string): RGBA | null {
  v = v.trim().toLowerCase()
  if (v === 'transparent') return [0, 0, 0, 0]
  let m = /^#([0-9a-f]{3,8})$/.exec(v)
  if (m) {
    let h = m[1]!
    if (h.length === 3 || h.length === 4) h = [...h].map((c) => c + c).join('')
    const n = (i: number) => parseInt(h.slice(i, i + 2), 16)
    return [n(0), n(2), n(4), h.length === 8 ? n(6) / 255 : 1]
  }
  m = /^rgba?\(\s*([\d.]+)[\s,]+([\d.]+)[\s,]+([\d.]+)(?:[\s,/]+([\d.]+%?))?\s*\)$/.exec(v)
  if (m) {
    const a = m[4] === undefined ? 1 : m[4].endsWith('%') ? parseFloat(m[4]) / 100 : parseFloat(m[4])
    return [+m[1]!, +m[2]!, +m[3]!, a]
  }
  return null
}

/** Alpha-composites fg over an opaque bg. */
export function over(fg: RGBA, bg: RGBA): RGBA {
  const a = fg[3]
  return [fg[0] * a + bg[0] * (1 - a), fg[1] * a + bg[1] * (1 - a), fg[2] * a + bg[2] * (1 - a), 1]
}

function channel(c: number) {
  const s = c / 255
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
}

export function luminance([r, g, b]: RGBA): number {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
}

export function contrast(a: RGBA, b: RGBA): number {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p) as [number, number]
  return (x + 0.05) / (y + 0.05)
}

/** Every token a theme must resolve (after shared defaults). */
export const REQUIRED_TOKENS = [
  '--scheme', '--bg', '--surface', '--surface-2', '--surface-raised', '--overlay', '--sidebar', '--sidebar-fg',
  '--sidebar-active', '--sidebar-active-fg', '--fg', '--muted', '--on-bg', '--on-bg-muted', '--border',
  '--input-border', '--accent', '--accent-fg', '--success', '--danger', '--danger-fg', '--warning', '--info',
  '--proto-ssh', '--proto-rdp', '--proto-vnc', '--badge-fg', '--focus', '--focus-halo', '--selection', '--elev-0', '--elev-1', '--elev-2',
  '--elev-3', '--elev-4', '--dur-1', '--dur-2', '--dur-3', '--ease-out', '--ease-in-out', '--motion-distance',
] // prettier-ignore

/** Text/background combinations the UI draws, with their WCAG 2.2 AAA minimum. */
export const PAIRS: Array<{ fg: string; bg: string; min: number; what: string }> = [
  { fg: '--fg', bg: '--surface', min: 7, what: 'body text on cards' },
  { fg: '--fg', bg: '--surface-raised', min: 7, what: 'body text on raised surfaces' },
  { fg: '--muted', bg: '--surface', min: 7, what: 'secondary text and labels' },
  { fg: '--muted', bg: '--surface-raised', min: 7, what: 'table headers' },
  { fg: '--on-bg', bg: '--bg', min: 7, what: 'page titles' },
  { fg: '--on-bg-muted', bg: '--bg', min: 7, what: 'page subtitles' },
  { fg: '--accent', bg: '--surface', min: 7, what: 'links, tabs and accent text' },
  { fg: '--accent-fg', bg: '--accent', min: 7, what: 'primary button text' },
  { fg: '--sidebar-fg', bg: '--sidebar', min: 7, what: 'sidebar items' },
  { fg: '--sidebar-active-fg', bg: '--sidebar-active', min: 7, what: 'active sidebar item' },
  { fg: '--success', bg: '--surface', min: 7, what: 'success messages' },
  { fg: '--danger', bg: '--surface', min: 7, what: 'error messages' },
  { fg: '--warning', bg: '--surface', min: 7, what: 'warnings' },
  { fg: '--danger-fg', bg: '--danger', min: 7, what: 'danger button text' },
  { fg: '--badge-fg', bg: '--proto-ssh', min: 7, what: 'SSH badge' },
  { fg: '--badge-fg', bg: '--proto-rdp', min: 7, what: 'RDP badge' },
  { fg: '--badge-fg', bg: '--proto-vnc', min: 7, what: 'VNC badge' },
  { fg: '--input-border', bg: '--surface', min: 3, what: 'input boundary' },
  { fg: '--focus', bg: '--surface', min: 3, what: 'focus indicator on cards' },
  { fg: '--focus', bg: '--bg', min: 3, what: 'focus indicator on the page' },
]

/** Opaque color of a token: translucent surfaces are composited over the page background. */
export function solid(tokens: Tokens, name: string): RGBA | null {
  const c = parseColor(resolve(tokens, name))
  if (!c) return null
  if (c[3] >= 1 || name === '--bg') return c
  const bg = parseColor(resolve(tokens, '--bg'))
  return bg ? over(c, over(bg, [255, 255, 255, 1])) : null
}

/**
 * Decorations that paint their own background (gradients, solid fills) and the
 * text color drawn on them: every color stop must reach 7:1 with that text.
 * Text color comes from the rule's `color`, or from the token the element uses.
 */
export function checkDecorations(css: string, tokens: Tokens): Failure[] {
  const out: Failure[] = []
  const implicit: Array<[RegExp, string]> = [
    [/:not\(\.in-client\) body$/, '--on-bg'],
    [/ aside$/, '--sidebar-fg'],
    [/ \.card-title$/, '--fg'],
    [/ \.btn-primary$/, '--accent-fg'],
    [/ \.btn$/, '--fg'],
  ]
  const re = /([^{}]+)\{([^{}]*)\}/g
  for (let m = re.exec(css); m; m = re.exec(css)) {
    const sel = m[1]!.replace(/\/\*[\s\S]*?\*\//g, '').trim()
    if (sel.includes('.theme-swatch') || /:hover|:active|::|\.selected/.test(sel)) continue
    const body = m[2]!
    const bg = /(?:^|;)\s*background(?:-color|-image)?\s*:\s*([^;]+)/.exec(body)?.[1]
    if (!bg) continue
    const stops = [...bg.matchAll(/#[0-9a-f]{3,8}\b|rgba?\([^)]*\)/gi)]
      .map((s) => parseColor(s[0]))
      .filter((c) => !!c) as RGBA[]
    if (!stops.length || stops.some((s) => s[3] < 1)) continue
    // the text sits on a child label with its own background (e.g. System 7 / twm title bars)
    const label = new RegExp(
      sel.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\s*>\\s*\\*\\s*\\{([^}]*)\\}',
    ).exec(css)
    if (label && /background\s*:\s*#[0-9a-f]{3,6}\b/i.test(label[1]!)) continue
    const colorDecl = /(?:^|;)\s*color\s*:\s*([^;]+)/.exec(body)?.[1]?.trim()
    const tokenName = implicit.find(([r]) => r.test(sel))?.[1]
    const fgText = colorDecl ?? (tokenName ? resolve(tokens, tokenName) : '')
    const fg = parseColor(fgText.startsWith('var(') ? resolve({ ...tokens, '--x': fgText }, '--x') : fgText)
    if (!fg) continue
    for (const s of stops) {
      const ratio = contrast(over(fg, s), s)
      if (ratio + 1e-9 < 7)
        out.push({
          what: `decoration ${sel}`,
          pair: `${fgText} on ${s.slice(0, 3).join(',')}`,
          ratio: +ratio.toFixed(2),
          min: 7,
        })
    }
  }
  return out
}

export interface Failure {
  what: string
  pair: string
  ratio: number
  min: number
}

export function checkContrast(tokens: Tokens): Failure[] {
  const out: Failure[] = []
  for (const p of PAIRS) {
    const bg = solid(tokens, p.bg)
    const fgRaw = parseColor(resolve(tokens, p.fg))
    if (!bg || !fgRaw) {
      out.push({ what: p.what, pair: `${p.fg} on ${p.bg} (unresolved)`, ratio: 0, min: p.min })
      continue
    }
    let ratio = contrast(over(fgRaw, bg), bg)
    if (p.fg === '--focus') {
      // a double ring passes when either ring stands out from what surrounds it
      const halo = parseColor(resolve(tokens, '--focus-halo'))
      if (halo && halo[3] > 0) ratio = Math.max(ratio, contrast(over(halo, bg), bg))
    }
    if (ratio + 1e-9 < p.min)
      out.push({ what: p.what, pair: `${p.fg} on ${p.bg}`, ratio: +ratio.toFixed(2), min: p.min })
  }
  return out
}
