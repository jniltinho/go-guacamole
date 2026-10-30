import { describe, expect, it } from 'vitest'

import { generatePassword } from './password'

describe('generatePassword', () => {
  it('has the length and every kind of character, never look-alikes', () => {
    for (let i = 0; i < 200; i++) {
      const p = generatePassword()
      expect(p).toHaveLength(16)
      expect(p).toMatch(/[A-Z]/)
      expect(p).toMatch(/[a-z]/)
      expect(p).toMatch(/[0-9]/)
      expect(p).toMatch(/[@#%+=!?\-_]/)
      expect(p).not.toMatch(/[Il1O0]/)
    }
    expect(generatePassword(24)).toHaveLength(24)
    expect(new Set(Array.from({ length: 50 }, () => generatePassword())).size).toBe(50)
  })
})
