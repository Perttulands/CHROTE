import { test, expect, allowBrowserConsoleMessage, type Route } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'
import { openSessionsSidecar } from './helpers'

test.describe('Error States', () => {
  async function fulfillMockSessions(route: Route) {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockSessions),
    })
  }

  test.describe('API 500 on session creation', () => {
    test('states the failure on the status line when POST /api/tmux/sessions returns 500', async ({ page }) => {
      allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 500')
      // Set up normal GET mock first, then override POST to fail
      await mockApiRoutes(page)
      await page.route('**/api/tmux/sessions', async route => {
        if (route.request().method() === 'POST') {
          await route.fulfill({
            status: 500,
            contentType: 'application/json',
            body: JSON.stringify({ error: 'tmux not running' }),
          })
        } else {
          await fulfillMockSessions(route)
        }
      })

      await page.goto('/')
      await page.waitForSelector('.dashboard')
      await openSessionsSidecar(page)
      await page.waitForSelector('.session-panel')

      // The launcher in an empty window is the only way to start a session
      const createBtn = page.locator('.launcher-launch').first()
      await expect(createBtn).toBeVisible()

      // Click it
      await createBtn.click()

      // Every announcement lands on the one status line, and nothing pops up
      // over the work: a failure is the only thing on it that takes colour.
      const status = page.locator('.status-line')
      await expect(status).toContainText('Failed to create session', { timeout: 5000 })
      await expect(status.locator('.status-line-failure')).toBeVisible()
      await expect(page.locator('.toast-item')).toHaveCount(0)
    })
  })

})
