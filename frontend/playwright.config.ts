import { defineConfig, devices } from '@playwright/test'

const chrome = process.env.PW_CHROME_PATH

export default defineConfig({
  testDir: './e2e',
  timeout: 90_000,
  retries: 0,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: process.env.GG_E2E_URL ?? 'http://127.0.0.1:18081',
    viewport: { width: 1440, height: 900 },
    trace: 'retain-on-failure',
  },
  projects: [
    // theme suite (screenshots + axe): GG_VISUAL=1 ... --project=visual
    ...(process.env.GG_VISUAL
      ? [
          {
            name: 'visual',
            testMatch: /themes\.spec\.ts/,
            use: {
              ...devices['Desktop Chrome'],
              viewport: { width: 1440, height: 900 },
              deviceScaleFactor: 1,
              locale: 'en-US',
              launchOptions: chrome ? { executablePath: chrome } : {},
            },
          },
        ]
      : []),
    {
      name: 'chromium',
      testIgnore: /themes\.spec\.ts/,
      use: { ...devices['Desktop Chrome'], launchOptions: chrome ? { executablePath: chrome } : {} },
    },
    ...(process.env.PW_FIREFOX === '0'
      ? []
      : [{ name: 'firefox', testIgnore: /themes\.spec\.ts/, use: { ...devices['Desktop Firefox'] } }]),
  ],
})
