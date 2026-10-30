import { describe, expect, it } from 'vite-plus/test'

import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

import {
  checkContrast,
  checkDecorations,
  contrast,
  parseColor,
  parseTokens,
  REQUIRED_TOKENS,
  resolve,
  type Tokens,
} from './contrast'

// CSS imports are empty under the test runner: read the files directly
const dir = fileURLToPath(new URL('.', import.meta.url))
const defaults = parseTokens(readFileSync(dir + 'base.css', 'utf8'), ':root')
const sheets = Object.fromEntries(
  readdirSync(dir)
    .filter((id) => existsSync(`${dir}${id}/theme.css`))
    .map((id) => [id, readFileSync(`${dir}${id}/theme.css`, 'utf8')]),
)

function themeTokens(id: string, css: string): Tokens {
  return { ...defaults, ...parseTokens(css, `html[data-theme='${id}']`) }
}

describe('contrast helpers', () => {
  it('matches WCAG reference values', () => {
    expect(contrast(parseColor('#000')!, parseColor('#fff')!)).toBeCloseTo(21, 5)
    expect(contrast(parseColor('#777777')!, parseColor('#ffffff')!)).toBeCloseTo(4.48, 2)
    expect(resolve({ '--a': 'var(--b, red)', '--b': '' }, '--a')).toBe('red')
  })
})

describe.each(Object.entries(sheets))('theme %s', (path, css) => {
  const tokens = themeTokens(path, css)

  it('resolves every required token (shared defaults included)', () => {
    const missing = REQUIRED_TOKENS.filter((t) => !resolve(tokens, t))
    expect(missing).toEqual([])
  })

  it('meets WCAG 2.2 AAA contrast for every text and boundary pair', () => {
    expect(checkContrast(tokens)).toEqual([])
  })

  it.each(['reduced', 'off'])('keeps AAA with the opaque surfaces of effects=%s', (fx) => {
    const sel = `html[data-theme='${path}'][data-effects='${fx}']`
    expect(checkContrast({ ...tokens, ...parseTokens(css, sel) })).toEqual([])
  })

  it('meets AAA on decorations that paint their own background', () => {
    expect(checkDecorations(css, tokens)).toEqual([])
  })

  it('never rounds corners nor styles the remote session', () => {
    expect(css).not.toMatch(/border-radius\s*:\s*(?!0[;\s}]|0px)/)
    const r = /--radius\s*:\s*(\d+)px/.exec(css)
    if (r) expect(Number(r[1])).toBeLessThanOrEqual(8)
    expect(css).not.toMatch(/\.client-view/)
    expect(css).not.toMatch(/html\[data-theme='[\w-]+'\] body/)
  })
})
