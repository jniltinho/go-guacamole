import { describe, expect, it } from 'vite-plus/test'

import en from '../locales/en.json'
import ptBR from '../locales/pt-BR.json'

function keys(obj: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(obj).flatMap(([k, v]) =>
    v && typeof v === 'object' ? keys(v as Record<string, unknown>, prefix + k + '.') : [prefix + k],
  )
}

describe('locale catalogs', () => {
  it('en and pt-BR have exactly the same keys', () => {
    const a = keys(en).sort()
    const b = keys(ptBR).sort()
    expect(a.filter((k) => !b.includes(k))).toEqual([])
    expect(b.filter((k) => !a.includes(k))).toEqual([])
  })

  it('has no empty translations', () => {
    const flat = (o: Record<string, unknown>): unknown[] =>
      Object.values(o).flatMap((v) => (v && typeof v === 'object' ? flat(v as Record<string, unknown>) : [v]))
    expect(flat(ptBR).filter((v) => v === '')).toEqual([])
  })
})
