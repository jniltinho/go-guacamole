export function duration(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(h)}:${pad(m)}:${pad(sec)}`
}

export function relative(t: (key: string, n: number) => string, at: number, now = Date.now()): string {
  const diff = Math.max(0, now - at) / 1000
  if (diff < 60) return t('duration.justNow', 0)
  if (diff < 3600) return t('duration.minutesAgo', Math.floor(diff / 60))
  if (diff < 86400) return t('duration.hoursAgo', Math.floor(diff / 3600))
  return t('duration.daysAgo', Math.floor(diff / 86400))
}
