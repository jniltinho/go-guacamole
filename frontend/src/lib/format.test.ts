import { describe, expect, it } from 'vite-plus/test'

import { duration } from './format'
import { detectLocale } from '../i18n/detect'

describe('format', () => {
  it('formats durations as HH:MM:SS', () => {
    expect(duration(0)).toBe('00:00:00')
    expect(duration(3_723_000)).toBe('01:02:03')
  })
})

describe('locale detection', () => {
  it('prefers saved locale, then browser language, then English', () => {
    expect(detectLocale('pt-BR', ['en-US'])).toBe('pt-BR')
    expect(detectLocale(null, ['pt-PT', 'en'])).toBe('pt-BR')
    expect(detectLocale(null, ['de-DE'])).toBe('en')
    expect(detectLocale('xx', [])).toBe('en')
  })
})

describe('human durations', () => {
  it('formats compactly', async () => {
    const { human } = await import('./format')
    expect(human(30_000)).toBe('30s')
    expect(human(45 * 60_000)).toBe('45m')
    expect(human((3 * 60 + 5) * 60_000)).toBe('3h 05m')
    expect(human(27 * 3600_000)).toBe('1d 3h')
  })
})
