// generatePassword makes a strong random password (as AD Manager's generator): at least one upper case letter, one
// lower case letter, one digit and one symbol, no look-alike characters (I l 1 O 0), from the browser's CSPRNG.
const SETS = ['ABCDEFGHJKLMNPQRSTUVWXYZ', 'abcdefghijkmnopqrstuvwxyz', '23456789', '@#%+=!?-_']
const ALL = SETS.join('')

// rnd is uniform in [0, max): values from the biased tail of 2^32 are drawn again
function rnd(max: number): number {
  const limit = Math.floor(0x100000000 / max) * max
  const buf = new Uint32Array(1)
  for (;;) {
    crypto.getRandomValues(buf)
    if (buf[0]! < limit) return buf[0]! % max
  }
}

export function generatePassword(length = 16): string {
  const s = SETS.map((g) => g[rnd(g.length)]!)
  while (s.length < length) s.push(ALL[rnd(ALL.length)]!)
  for (let i = s.length - 1; i > 0; i--) {
    const j = rnd(i + 1)
    ;[s[i], s[j]] = [s[j]!, s[i]!]
  }
  return s.join('')
}
