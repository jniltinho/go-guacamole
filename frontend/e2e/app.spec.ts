import { expect, test, type APIRequestContext, type Page } from '@playwright/test'

const ADMIN = process.env.GG_E2E_ADMIN_USER ?? 'admin'
const PASS = process.env.GG_E2E_ADMIN_PASSWORD ?? 'adminadmin1'
const NEW_PASS = 'Browser-E2E#2026'

async function apiLogin(request: APIRequestContext, baseURL: string) {
  const headers = { 'X-Requested-With': 'goguacamole', Origin: baseURL }
  for (const body of [
    { username: ADMIN, password: PASS },
    { username: ADMIN, password: PASS, newPassword: NEW_PASS },
    { username: ADMIN, password: NEW_PASS },
  ]) {
    const r = await request.post('/api/v1/auth/login', { data: body, headers })
    if (r.ok()) return { ...headers, 'X-CSRF-Token': (await r.json()).csrfToken }
  }
  throw new Error('admin login failed')
}

async function ensureConnection(
  request: APIRequestContext,
  h: Record<string, string>,
  name: string,
  body: object,
) {
  const list = await (await request.get('/api/v1/connections', { params: { q: name }, headers: h })).json()
  const found = list.items.find((c: { name: string }) => c.name === name)
  if (found) return found.id as string
  const r = await request.post('/api/v1/connections', { data: { name, ...body }, headers: h })
  expect(r.status(), await r.text()).toBe(201)
  return (await r.json()).id as string
}

async function uiLogin(page: Page) {
  await page.goto('/login')
  await page.locator('#u').fill(ADMIN)
  await page.locator('#p').fill(NEW_PASS)
  await page.getByRole('button', { name: /sign in|entrar/i }).click()
  await expect(page).toHaveURL(/dashboard/)
}

const conns: Record<string, string> = {}

test.beforeAll(async ({ request, baseURL }) => {
  const h = await apiLogin(request, baseURL!)
  conns.ssh = await ensureConnection(request, h, 'e2e-ssh', {
    protocol: 'ssh',
    parameters: { hostname: 'ssh-target', port: '2222', username: 'lab', password: 'labpass123' },
  })
  conns.vnc = await ensureConnection(request, h, 'e2e-vnc', {
    protocol: 'vnc',
    parameters: { hostname: 'vnc-target', port: '5901', password: 'labpass123' },
  })
  conns.rdp = await ensureConnection(request, h, 'e2e-rdp', {
    protocol: 'rdp',
    parameters: {
      hostname: 'rdp-target',
      port: '3389',
      username: 'ubuntu',
      password: 'ubuntu',
      security: 'any',
      'ignore-cert': 'true',
    },
  })
})

test('login in Portuguese and English', async ({ browser, baseURL }) => {
  const pt = await browser.newContext({ locale: 'pt-BR', baseURL })
  const p = await pt.newPage()
  await p.goto('/login')
  await expect(p.getByRole('button', { name: 'Entrar' })).toBeVisible()
  await pt.close()
  const en = await browser.newContext({ locale: 'en-US', baseURL })
  const e = await en.newPage()
  await e.goto('/login')
  await expect(e.getByRole('button', { name: 'Sign in' })).toBeVisible()
  await en.close()
})

test('dashboard shows real counts', async ({ page }) => {
  await uiLogin(page)
  await expect(page.getByRole('heading', { name: /dashboard|painel/i })).toBeVisible()
  await expect(page.getByTestId('stat-value').first()).toHaveText(/\d+/)
})

for (const proto of ['ssh', 'vnc', 'rdp'] as const) {
  test(`${proto.toUpperCase()} session renders a remote display`, async ({ page }) => {
    await uiLogin(page)
    await page.goto(`/sessions/${conns[proto]}/client`)
    const canvas = page.getByTestId('client-viewport').locator('canvas').first()
    await expect(canvas).toBeVisible({ timeout: 45_000 })
    await expect(page.getByTestId('client-error')).toHaveCount(0)
    // resize is sent without breaking the session
    await page.setViewportSize({ width: 1200, height: 760 })
    await page.waitForTimeout(800)
    await expect(page.getByTestId('client-error')).toHaveCount(0)
    if (proto === 'ssh') {
      await page.getByTestId('client-viewport').click()
      await page.keyboard.type('echo e2e-ok', { delay: 20 })
      await page.keyboard.press('Enter')
      await page.getByTestId('open-menu').click()
      await page.getByTestId('clipboard').fill('from-browser')
      await page.getByRole('button', { name: /send to remote|enviar ao remoto/i }).click()
    }
    await page.getByTestId('open-menu').or(page.getByTestId('disconnect')).first().click()
    await page.getByTestId('disconnect').click()
    await expect(page).toHaveURL(/connections/)
  })
}

test('theme choice persists across reloads', async ({ page }) => {
  await uiLogin(page)
  await page.getByTestId('theme-picker').first().selectOption('dracula')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dracula')
  await page.getByTestId('theme-picker').first().selectOption('light')
})
