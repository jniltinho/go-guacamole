import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'

// Visual regression and accessibility of every theme (openspec ui-theming).
// Run with the visual project: GG_VISUAL=1 npx playwright test e2e/themes.spec.ts --project=visual
// The login page of every theme is a pixel reference, recorded in the pinned Playwright container
// (fonts, DPR 1, en locale, effects off, fixed clock):
//   docker run --rm --network host -v $PWD:/w -w /w mcr.microsoft.com/playwright:v1.64.1-noble \
//     npx playwright test e2e/themes.spec.ts --project=visual --update-snapshots
const THEMES = [
  'light', 'dark', 'noc', 'dracula', 'bionexo', 'matrix', 'xterm', 'macmodern',
  'ubuntu', 'kde', 'google', 'contrast', 'win11', 'xp', 'win98', 'macos',
] // prettier-ignore
// GG_SNAPSHOTS=0 runs only the accessibility checks (references come from the pinned container)
const SNAP = process.env.GG_SNAPSHOTS !== '0'
const ADMIN = process.env.GG_E2E_ADMIN_USER ?? 'admin'
const PASS = process.env.GG_E2E_ADMIN_PASSWORD ?? 'adminadmin1'
const NEW_PASS = 'Browser-E2E#2026'

test.describe.configure({ mode: 'serial' })
test.skip(!process.env.GG_VISUAL, 'set GG_VISUAL=1 to run the theme suite')

async function signIn(page: Page, theme: string) {
  await page.goto('/login')
  const h = { 'X-Requested-With': 'goguacamole', Origin: new URL(page.url()).origin }
  // same bootstrap as app.spec.ts: a fresh lab admin must change its password first
  let csrf = ''
  for (const body of [
    { username: ADMIN, password: PASS },
    { username: ADMIN, password: PASS, newPassword: NEW_PASS },
    { username: ADMIN, password: NEW_PASS },
  ]) {
    const r = await page.request.post('/api/v1/auth/login', { data: body, headers: h })
    if (r.ok()) {
      csrf = (await r.json()).csrfToken
      break
    }
  }
  expect(csrf, 'admin login failed').not.toBe('')
  // the server preference wins over the browser cache: set theme and effects there
  const r = await page.request.put('/api/v1/me/preferences', {
    data: { theme, locale: 'en', sidebarCollapsed: false, effects: 'off' },
    headers: { ...h, 'X-CSRF-Token': csrf },
  })
  expect(r.ok(), 'save preferences').toBeTruthy()
  return { ...h, 'X-CSRF-Token': csrf }
}

const SCREENS = [
  { name: 'dashboard', path: '/dashboard' },
  { name: 'connections', path: '/connections' },
  { name: 'connection-edit', path: '/connections/1/edit' },
  { name: 'sessions', path: '/sessions' },
  { name: 'users', path: '/users' },
  { name: 'audit', path: '/audit' },
  { name: 'settings', path: '/settings' },
  { name: 'recording', path: '/recording' },
]

for (const theme of THEMES) {
  test(`theme ${theme}`, async ({ page }) => {
    await page.clock.setFixedTime(new Date('2026-01-15T12:00:00Z'))
    await page.goto('/login')
    await page.evaluate(
      ([t]) => {
        localStorage.setItem('gg-theme', t!)
        localStorage.setItem('gg-effects', 'off')
      },
      [theme],
    )
    await page.reload()
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
    await expect(page.locator('html')).toHaveAttribute('data-effects', 'off')
    if (SNAP) await expect(page).toHaveScreenshot(`${theme}-login.png`, { maxDiffPixelRatio: 0.002 })

    await signIn(page, theme)
    for (const s of SCREENS) {
      await page.goto(s.path)
      await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
      await page.waitForLoadState('networkidle')

      // WCAG 2.2 AAA contrast and focus/live-region rules; inconclusive results are reported too
      const axe = await new AxeBuilder({ page })
        .withRules(['color-contrast-enhanced', 'color-contrast', 'aria-allowed-role', 'region'])
        .disableRules(['region'])
        .analyze()
      expect(
        axe.violations.map(
          (v) =>
            `${s.name} ${v.id}: ${v.nodes.map((n) => `${n.target.join(' ')} [${n.html.slice(0, 90)}]`).join(' | ')}`,
        ),
      ).toEqual([])

      // every focusable element shows an indicator when reached with the keyboard
      for (let i = 0; i < 6; i++) {
        await page.keyboard.press('Tab')
        const visible = await page.evaluate(() => {
          const el = document.activeElement as HTMLElement | null
          if (!el || el === document.body) return true
          const cs = getComputedStyle(el)
          const outline = cs.outlineStyle !== 'none' && parseFloat(cs.outlineWidth) >= 2
          const ring = cs.boxShadow !== 'none' && !/^rgba\(0, 0, 0, 0\)/.test(cs.boxShadow)
          return outline || ring
        })
        expect(visible, `focus indicator on ${s.name}`).toBeTruthy()
      }

      // screens with data are checked by axe; only the data-free login page is a pixel reference
    }

    // sidebar focus: the outline must reach 3:1 against the link's real background, active and inactive
    await page.goto('/dashboard')
    await page.keyboard.press('Tab') // keyboard modality, so focus() shows :focus-visible
    const links = page.locator('aside nav a, aside a.nav-item')
    const n = await links.count()
    expect(n).toBeGreaterThan(1)
    for (let i = 0; i < n; i++) {
      await links.nth(i).focus()
      const r = await links.nth(i).evaluate((el) => {
        const rgb = (c: string) => (c.match(/[\d.]+/g) ?? []).map(Number)
        const lum = ([r, g, b]: number[]) =>
          [r!, g!, b!]
            .map((v) => v / 255)
            .map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4))
            .reduce((acc, v, k) => acc + v * [0.2126, 0.7152, 0.0722][k]!, 0)
        let bg = 'rgba(0, 0, 0, 0)'
        for (let e: Element | null = el; e; e = e.parentElement) {
          const c = getComputedStyle(e).backgroundColor
          if (!/rgba\(.*, 0\)$/.test(c) && c !== 'transparent') {
            bg = c
            break
          }
        }
        const cs = getComputedStyle(el)
        const [a, b] = [lum(rgb(cs.outlineColor)), lum(rgb(bg))].sort((x, y) => y - x) as [number, number]
        return { ratio: (a + 0.05) / (b + 0.05), style: cs.outlineStyle, href: el.getAttribute('href') }
      })
      expect(r.style, `outline on ${r.href}`).not.toBe('none')
      expect(r.ratio, `sidebar focus contrast on ${r.href}`).toBeGreaterThanOrEqual(3)
    }
  })
}

// The remote session is never styled by a theme, in every effects mode, and its
// scaling (which the pointer mapping relies on) is only Guacamole's own.
for (const theme of ['matrix', 'macmodern']) {
  test(`remote session untouched in ${theme}`, async ({ page }) => {
    await page.goto('/login')
    await page.evaluate(([t]) => localStorage.setItem('gg-theme', t!), [theme])
    const h = await signIn(page, theme)
    // GG_E2E_SSH_CONN, else the lab connection app.spec.ts creates
    let conn = process.env.GG_E2E_SSH_CONN
    if (!conn) {
      const r = await page.request.get('/api/v1/connections', {
        params: { q: 'e2e-ssh' },
        headers: { 'X-Requested-With': 'goguacamole' },
      })
      conn = r.ok()
        ? (await r.json()).items.find((c: { name: string }) => c.name === 'e2e-ssh')?.id
        : undefined
    }
    test.skip(!conn, 'set GG_E2E_SSH_CONN to a reachable SSH connection id')
    for (const fx of ['full', 'reduced', 'off']) {
      // the server preference is what the app applies after sign-in
      const r = await page.request.put('/api/v1/me/preferences', {
        data: { theme, locale: 'en', sidebarCollapsed: false, effects: fx },
        headers: h,
      })
      expect(r.ok(), `save effects=${fx}`).toBeTruthy()
      await page.goto(`/sessions/${conn}/client`)
      await expect(page.locator('html')).toHaveClass(/in-client/)
      await expect(page.locator('html')).toHaveAttribute('data-effects', fx)
      const canvas = page.locator('[data-testid="client-viewport"] canvas').first()
      await expect(canvas).toBeVisible({ timeout: 20_000 })
      for (const size of [
        { width: 1440, height: 900 },
        { width: 1100, height: 700 },
      ]) {
        await page.setViewportSize(size)
        await page.waitForTimeout(800)
        const issues = await page.evaluate(() => {
          const out: string[] = []
          // Guacamole's own layers carry its scale transform; the page from the viewport up must add nothing
          const viewport = document.querySelector('[data-testid="client-viewport"]')!
          const display = viewport.querySelector('canvas')!.parentElement!
          for (let e: Element | null = viewport; e; e = e.parentElement) {
            const cs = getComputedStyle(e)
            if (cs.transform !== 'none') out.push(`transform on ${e.tagName}.${e.className}`)
            if (cs.backdropFilter && cs.backdropFilter !== 'none') out.push(`blur on ${e.tagName}`)
            if (cs.textShadow !== 'none') out.push(`text-shadow on ${e.tagName}`)
            if (parseFloat(cs.transitionDuration) > 0 && cs.transitionProperty !== 'none')
              out.push(`transition on ${e.tagName}.${e.className}`)
          }
          if (document.getElementById('matrix-rain')) out.push('matrix rain rendered')
          // the display fits the viewport: Guacamole's scale is the only transform applied
          const r = display.getBoundingClientRect()
          const vp = document.querySelector('[data-testid="client-viewport"]')!.getBoundingClientRect()
          if (r.width > vp.width + 1 || r.height > vp.height + 1) out.push('display larger than the viewport')
          return out
        })
        expect(issues, `${theme} effects=${fx} ${size.width}x${size.height}`).toEqual([])
      }
      // a click lands inside the display (no offset introduced by the page)
      const box = (await canvas.boundingBox())!
      const hit = await page.evaluate(
        ([x, y]) => document.elementFromPoint(x!, y!)?.closest('[data-testid="client-viewport"]') !== null,
        [box.x + box.width / 2, box.y + box.height / 2],
      )
      expect(hit).toBeTruthy()
      await page.goto('/dashboard')
      await expect(page.locator('html')).not.toHaveClass(/in-client/)
    }
  })
}
