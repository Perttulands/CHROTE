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
    test('should show error toast when POST /api/tmux/sessions returns 500', async ({ page }) => {
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

      // Find the "New Session" button in an empty window
      const createBtn = page.locator('.create-session-btn').first()
      await expect(createBtn).toBeVisible()

      // Click it
      await createBtn.click()

      // Error toast should appear
      const toast = page.locator('.toast-item.toast-error')
      await expect(toast).toBeVisible({ timeout: 5000 })
      await expect(toast.locator('.toast-message')).toContainText('Failed to create session')
    })
  })

})
