import { defineConfig, devices } from '@playwright/test'

const liveBackend = process.env.CHROTE_PLAYWRIGHT_LIVE === '1'
const devServerURL = 'http://localhost:5173'
const liveBackendURL = process.env.CHROTE_TEST_URL ?? 'http://127.0.0.1:8094'

export default defineConfig({
  testDir: './tests',
  testIgnore: liveBackend ? [] : ['**/integration/**'],
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: liveBackend ? liveBackendURL : devServerURL,
    trace: 'on-first-retry',
  },
  projects: liveBackend ? [
    {
      name: 'live-backend',
      use: { ...devices['Desktop Chrome'] },
    },
  ] : [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
      testIgnore: ['**/mobile.spec.ts', '**/integration/**'],
    },
    {
      name: 'mobile',
      use: {
        ...devices['iPhone 13'],
        defaultBrowserType: 'chromium',
      },
      testMatch: 'tests/mobile.spec.ts',
    },
  ],
  webServer: liveBackend ? undefined : {
    command: 'CHROTE_PLAYWRIGHT_MOCKED=1 npm run dev',
    url: devServerURL,
    reuseExistingServer: !process.env.CI,
    timeout: 120000,
  },
})
