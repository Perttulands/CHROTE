import { test, expect, Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

async function openDashboardHelp(page: Page) {
  await page.locator('.help-menu-container > .tab').click()
  await page.locator('.help-dropdown-item', { hasText: 'Dashboard Help' }).click()
  await page.waitForSelector('.help-view')
}

test.describe('Dashboard Help View', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test('opens Dashboard Help from the help menu', async ({ page }) => {
    await openDashboardHelp(page)

    await expect(page.locator('.help-title')).toContainText('Dashboard Help')
    await expect(page.locator('.help-subtitle')).toContainText('How to use this interface.')
    await expect(page.locator('.session-panel')).not.toBeVisible()
    await expect(page.locator('.help-nav-item')).toHaveCount(5)
  })

  test('shows current CHROTE help sections', async ({ page }) => {
    await openDashboardHelp(page)

    await expect(page.locator('.help-nav-item:has-text("Shortcuts")')).toHaveClass(/active/)
    await expect(page.locator('.help-section-content h2')).toContainText('Keyboard Shortcuts')

    await page.click('.help-nav-item:has-text("Terminals")')
    await expect(page.locator('.help-section-content h2')).toContainText('Terminal Panes')

    await page.click('.help-nav-item:has-text("Sessions")')
    await expect(page.locator('.help-section-content h2')).toContainText('Session Panel')

    await page.click('.help-nav-item:has-text("Files")')
    await expect(page.locator('.help-section-content h2')).toContainText('File Browser')

    await page.click('.help-nav-item:has-text("tmux")')
    await expect(page.locator('.help-section-content h2')).toContainText('tmux Reference')
  })

  test('returns to the terminal workspace from Dashboard Help', async ({ page }) => {
    await openDashboardHelp(page)
    await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()

    await expect(page.locator('.session-panel')).toBeVisible()
    await expect(page.locator('.terminal-area:visible')).toBeVisible()
  })
})
