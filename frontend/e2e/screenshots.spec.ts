import { expect, test, type Page } from '@playwright/test'

// README screenshots (docs/screenshots), taken on a clean lab stack: ./deploy/e2e.sh screenshots
// Demo data is created through the API; LAB-SSH (real) is the lab SSH target, the others are offline examples.
const ADMIN = process.env.GG_E2E_ADMIN_USER ?? 'admin'
const PASS = process.env.GG_E2E_ADMIN_PASSWORD ?? 'adminadmin1'
const NEW_PASS = 'Browser-E2E#2026'
const OUT = '.shots/'

test.describe.configure({ mode: 'serial' })
test.skip(!process.env.GG_SHOTS, 'set GG_SHOTS=1 to take the README screenshots')
test.use({ viewport: { width: 1920, height: 1080 } })

let h: Record<string, string> = {}

async function api(page: Page, method: string, url: string, data?: object) {
  const r = await page.request.fetch('/api/v1' + url, { method, data, headers: h })
  expect(r.ok(), `${method} ${url}: ${await r.text()}`).toBeTruthy()
  return r.status() === 204 ? {} : r.json()
}

async function signIn(page: Page) {
  await page.goto('/login')
  const base = { 'X-Requested-With': 'goguacamole', Origin: new URL(page.url()).origin }
  for (const body of [
    { username: ADMIN, password: PASS },
    { username: ADMIN, password: PASS, newPassword: NEW_PASS },
    { username: ADMIN, password: NEW_PASS },
  ]) {
    const r = await page.request.post('/api/v1/auth/login', { data: body, headers: base })
    if (r.ok()) {
      h = { ...base, 'X-CSRF-Token': (await r.json()).csrfToken }
      return
    }
  }
  throw new Error('admin login failed')
}

async function prefs(page: Page, theme: string, locale: string) {
  await api(page, 'PUT', '/me/preferences', { theme, locale, sidebarCollapsed: false, effects: 'off' })
}

async function shot(page: Page, path: string, name: string) {
  await page.goto(path)
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(600)
  await page.screenshot({ path: OUT + name })
}

test('README screenshots', async ({ page }) => {
  test.setTimeout(300_000)
  await signIn(page)

  // demo data: folders, connections, users and groups
  const folder: Record<string, string> = {}
  for (const name of ['Lab', 'Databases', 'Servers'])
    folder[name] = (await api(page, 'POST', '/connection-groups', { name })).id
  const conn = (name: string, protocol: string, parentId: string, parameters: object) =>
    api(page, 'POST', '/connections', { name, protocol, parentId, parameters })
  const ssh = (await conn('LAB-SSH (real)', 'ssh', folder.Lab!, {
    hostname: 'ssh-target', port: '2222', username: 'lab', password: 'labpass123',
  })).id // prettier-ignore
  await conn('LAB-VNC-01', 'vnc', folder.Lab!, { hostname: '192.168.56.10', port: '5900' })
  const db = (await conn('SRV-DB-01', 'ssh', folder.Databases!, { hostname: '192.168.10.30', port: '22' })).id
  const linux = (
    await conn('SRV-LINUX-01', 'ssh', folder.Servers!, { hostname: '192.168.10.10', port: '22' })
  ).id
  const win = (
    await conn('SRV-WINDOWS-02', 'rdp', folder.Servers!, { hostname: '192.168.10.20', port: '3389' })
  ).id
  const user = async (username: string, fullName: string) =>
    (
      await api(page, 'POST', '/users', {
        username,
        fullName,
        password: 'Demo-Pass#2026',
        mustChangePassword: false,
      })
    ).id
  const [alice, bob, charlie] = [
    await user('alice', 'Alice Martins'),
    await user('bob.smith', 'Bob Smith'),
    await user('charlie', 'Charlie Souza'),
  ]
  const group = async (name: string, members: string[], connections: string[]) => {
    const id = (await api(page, 'POST', '/user-groups', { name })).id
    await api(page, 'PUT', `/user-groups/${id}/members`, { members })
    await api(page, 'PUT', '/permissions', { subject: `group:${id}`, connections })
  }
  await group('Developers', [alice, charlie], [ssh, linux])
  await group('IT Ops', [bob], [ssh, db, linux, win])

  // a real SSH session: the client with its menu, then history and recording
  await prefs(page, 'light', 'en')
  await page.goto(`/sessions/${ssh}/client`)
  await expect(page.getByTestId('client-viewport').locator('canvas').first()).toBeVisible({ timeout: 45_000 })
  await page.waitForTimeout(1500)
  await page.getByTestId('client-viewport').click()
  await page.keyboard.type('uname -a && ls /', { delay: 30 })
  await page.keyboard.press('Enter')
  await page.waitForTimeout(1500)
  await page.getByTestId('open-menu').click()
  await page.waitForTimeout(400)
  await page.screenshot({ path: OUT + 'session-ssh.png' })
  await page.getByTestId('disconnect').click()
  await expect(page).toHaveURL(/connections/)

  // reachability is probed in the background (every 60s): wait until every status is known
  await expect
    .poll(
      async () =>
        (await api(page, 'GET', '/dashboard')).connections.every(
          (c: { status: string }) => c.status !== 'unknown',
        ),
      {
        timeout: 90_000,
        intervals: [3000],
      },
    )
    .toBeTruthy()
  await shot(page, '/connections', 'connections.png')
  for (const [theme, locale, name] of [
    ['light', 'pt-BR', 'dashboard-light.png'],
    ['win98', 'en', 'dashboard-win98.png'],
    ['dracula', 'en', 'dashboard-dracula.png'],
    ['noc', 'en', 'dashboard-noc.png'],
  ] as const) {
    await prefs(page, theme, locale)
    await shot(page, '/dashboard', name)
  }

  await prefs(page, 'light', 'en')
  await page.setViewportSize({ width: 1600, height: 900 })
  await shot(page, '/settings', 'settings.png')
  await page.goto('/sessions')
  await page.getByRole('tab', { name: /history/i }).click()
  await page.getByTestId('play-recording').first().click()
  await expect(page.getByTestId('recording-display').locator('canvas').first()).toBeAttached({
    timeout: 20_000,
  })
  await page.waitForTimeout(3000)
  await page.screenshot({ path: OUT + 'recording-player.png' })

  // login in the Windows XP theme (signed out, theme from the browser)
  await page.setViewportSize({ width: 1920, height: 1080 })
  await api(page, 'POST', '/auth/logout')
  await page.goto('/login')
  await page.evaluate(() => {
    localStorage.setItem('gg-theme', 'xp')
    localStorage.setItem('gg-effects', 'off')
    localStorage.setItem('gg-locale', 'en')
  })
  await page.reload()
  await page.waitForTimeout(600)
  await page.screenshot({ path: OUT + 'login-xp.png' })
})
