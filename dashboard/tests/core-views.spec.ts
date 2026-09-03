import { expect, test } from './fixtures'
import { mockApiRoutes } from './mock-api'

function envelope(data: object) {
  return JSON.stringify({ success: true, data, timestamp: '2026-08-29T12:00:00Z' })
}

test('opens Dashboard Help from the help menu', async ({ page }) => {
  await mockApiRoutes(page)
  await page.goto('/')
  await page.waitForSelector('.dashboard')

  await page.locator('.keys-menu-container > .tab').click({ button: 'right' })
  await page.locator('.menu-row', { hasText: 'Dashboard Help' }).click()

  await expect(page.locator('.help-title')).toContainText('Dashboard Help')
  await expect(page.locator('.help-subtitle')).toContainText('How to use this interface.')
  await expect(page.locator('.help-nav-item')).toHaveCount(5)
  for (const [tab, heading] of [
    ['Terminals', 'Terminal Panes'],
    ['Sessions', 'Session Sidecar'],
    ['Files', 'File Browser'],
    ['tmux', 'tmux Reference'],
  ] as const) {
    await page.locator('.help-nav-item', { hasText: tab }).click()
    await expect(page.locator('.help-section-content h2')).toContainText(heading)
  }

  await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()
  await expect(page.locator('.terminal-area:visible')).toBeVisible()
})

test('opens Scheduled and runs the selected task now', async ({ page }) => {
  await mockApiRoutes(page)
  const task = {
    id: 'tsk_existing',
    name: 'Continue work',
    prompt: 'Continue if work is clear',
    targets: [{ sessionName: 'hq-mayor' }],
    schedule: { type: 'cron', expression: '0 16 * * *', timezone: 'Europe/Helsinki' },
    enabled: true,
    paused: false,
    nextRun: '2026-08-30T13:00:00Z',
    createdBy: 'user:dashboard',
    updatedBy: 'user:dashboard',
  }
  let ranNow = false
  await page.route('**/api/scheduled-tasks**', async route => {
    const request = route.request()
    if (request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/tsk_existing/run-now')) {
      ranNow = true
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: envelope({ task, run: { id: 'run_now', status: 'success', targets: [{ sessionName: 'hq-mayor', status: 'success' }] } }),
      })
      return
    }
    await route.fulfill({ status: 200, contentType: 'application/json', body: envelope({ tasks: [task] }) })
  })

  await page.goto('/')
  await page.locator('.tab-bar-tabs .tab').filter({ hasText: /^Scheduled$/ }).click()
  await expect(page.getByRole('heading', { name: 'Scheduled Tasks' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Continue work' })).toBeVisible()

  await page.getByRole('button', { name: 'Run now' }).click()
  await expect.poll(() => ranNow).toBe(true)
  await expect(page.getByText('Sent to 1 session.', { exact: true })).toBeVisible()
})

test('opens Services and enqueues a TTS message', async ({ page }) => {
  await mockApiRoutes(page)
  await page.addInitScript(() => {
    Object.defineProperty(window, 'EventSource', { configurable: true, value: undefined })
  })
  let enqueueBody: unknown
  await page.route('**/api/services**', async route => {
    const request = route.request()
    const pathname = new URL(request.url()).pathname
    if (pathname === '/api/services') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: envelope({ services: [
          { id: 'tts', name: 'TTS Gateway', status: 'configured', configured: true, capabilities: [] },
          { id: 'context', name: 'Context Citadel', status: 'degraded', configured: false, tokenConfigured: false, capabilities: [] },
        ] }),
      })
      return
    }
    if (pathname === '/api/services/tts/enqueue') {
      enqueueBody = request.postDataJSON()
      await route.fulfill({ status: 202, contentType: 'application/json', body: envelope({ id: 'queued1', status: 'queued' }) })
      return
    }
    const data = pathname === '/api/services/tts/health'
      ? { status: 'ok', messages: 0, clients: 0 }
      : pathname === '/api/services/tts/messages'
        ? { messages: [] }
        : { docs: [] }
    await route.fulfill({ status: 200, contentType: 'application/json', body: envelope(data) })
  })

  await page.goto('/')
  await page.locator('.tab-bar-tabs .tab').filter({ hasText: /^Services$/ }).click()
  await expect(page.getByRole('heading', { name: 'TTS Gateway' })).toBeVisible()

  await page.getByLabel('TTS text').fill('Short spoken status.')
  await page.getByRole('button', { name: 'Enqueue' }).click()
  await expect.poll(() => enqueueBody).toMatchObject({
    text: 'Short spoken status.',
    source: 'Codex',
    backend: 'edge',
    voice: 'en-US-ChristopherNeural',
  })
  await expect(page.getByText('queued1 queued')).toBeVisible()
})
