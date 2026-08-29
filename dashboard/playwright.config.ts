import { defineConfig, devices } from '@playwright/test'

const liveBackend = process.env.CHROTE_PLAYWRIGHT_LIVE === '1'
const serverContract = process.env.CHROTE_PLAYWRIGHT_SERVER_CONTRACT === '1'
const externalServer = liveBackend || serverContract
const devServerPort = process.env.CHROTE_PLAYWRIGHT_PORT ?? '5173'
if (!/^\d+$/.test(devServerPort)) {
  throw new Error(`CHROTE_PLAYWRIGHT_PORT must be numeric, got ${devServerPort}`)
}
const devServerURL = `http://127.0.0.1:${devServerPort}`
const configuredBackendURL = process.env.CHROTE_TEST_URL
if (serverContract && !configuredBackendURL) {
  throw new Error('CHROTE_TEST_URL is required for the built-server contract; use scripts/test-built-server-contract.sh')
}
const liveBackendURL = configuredBackendURL ?? 'http://127.0.0.1:8095'
const reuseExistingDevServer = process.env.CHROTE_PLAYWRIGHT_REUSE_SERVER === '1'
const localOnlyIgnores = [
  '**/integration/**',
  '**/contract/**',
]

export default defineConfig({
  testDir: './tests',
  testIgnore: externalServer ? [] : localOnlyIgnores,
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  // Undefined workers means half the host's cores; on a shared 16-core host two
  // overlapping runs (each 8 Chromes) starve each other into beforeEach timeouts.
  workers: process.env.CI ? 1 : 4,
  // Key artifacts on CHROTE_PLAYWRIGHT_PORT so concurrent runs in one checkout do
  // not wipe each other's outputDir (Playwright removes it at every run start).
  // The external-server modes start no Vite server but still key on the same var.
  outputDir: `test-results/port-${devServerPort}`,
  reporter: [['html', { outputFolder: `playwright-report/port-${devServerPort}` }]],
  use: {
    baseURL: externalServer ? liveBackendURL : devServerURL,
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: serverContract ? [
    {
      name: 'server-contract',
      use: { ...devices['Desktop Chrome'] },
      testMatch: 'contract/**/*.spec.ts',
    },
  ] : liveBackend ? [
    {
      name: 'live-backend',
      use: { ...devices['Desktop Chrome'] },
    },
  ] : [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
      testIgnore: ['**/mobile.spec.ts', '**/integration/**', '**/contract/**'],
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
  webServer: externalServer ? undefined : {
    command: `CHROTE_PLAYWRIGHT_MOCKED=1 npm run dev -- --host 127.0.0.1 --port ${devServerPort} --strictPort`,
    url: devServerURL,
    reuseExistingServer: reuseExistingDevServer,
    timeout: 120000,
  },
})
