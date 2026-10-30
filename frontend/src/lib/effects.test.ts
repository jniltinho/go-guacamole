import { describe, expect, it } from 'vite-plus/test'

import { resolveEffects } from './effects'

describe('resolveEffects', () => {
  it('uses the explicit choice', () => {
    expect(resolveEffects('off', false)).toBe('off')
    expect(resolveEffects('full', true)).toBe('full')
  })
  it('follows the OS when nothing was chosen', () => {
    expect(resolveEffects('', true)).toBe('reduced')
    expect(resolveEffects(null, false)).toBe('full')
    expect(resolveEffects('extreme', false)).toBe('full')
  })
})
