import { test, expect, Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

// Extended mock for settings tests
async function mockSettingsApiRoutes(page: Page) {
  await mockApiRoutes(page)

  // Mock appearance endpoint
  await page.route('**/api/tmux/appearance', async route => {
    const request = route.request()
    const body = request.postDataJSON()

    // Validate the payload structure
    if (body && typeof body === 'object') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          applied: 2,
          total: 2,
          timestamp: new Date().toISOString(),
        }),
      })
    } else {
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, error: 'Invalid payload' }),
      })
    }
  })
}

test.describe('Settings View', () => {
  test.beforeEach(async ({ page }) => {
    await mockSettingsApiRoutes(page)
    // Clear persisted dashboard state before the app boots. The session flag keeps
    // reload-based persistence checks from wiping the state they just wrote.
    await page.addInitScript(() => {
      const clearFlag = '__chrote_settings_storage_cleared'
      if (sessionStorage.getItem(clearFlag) === '1') return
      localStorage.clear()
      sessionStorage.setItem(clearFlag, '1')
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test.describe('Theme Selection', () => {

    test('should switch to Settings view', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')
      await expect(page.locator('.settings-view')).toBeVisible()
    })

    test('should change theme to Matrix', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')

      // Find Matrix theme option and click it
      const matrixOption = page.locator('.theme-option.theme-matrix').first()
      await matrixOption.click()

      // Verify data-theme attribute changes on document
      const dataTheme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
      expect(dataTheme).toBe('matrix')
    })

  })

})
