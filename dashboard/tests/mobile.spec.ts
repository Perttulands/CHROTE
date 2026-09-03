import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'

// The 'mobile' project in playwright.config.ts runs this file, and only this
// file, under iPhone 13 device emulation.

test.describe('Terminal Area - Mobile View Switcher', () => {
  // A phone has no keyboard to reach the chords with, so the hamburger is the
  // only way to every control on this page: the pager between windows, the
  // count that pager reads, and the switch to another tab.
  test('pages between windows, changes the count through the tab menu, and switches tab', async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    const viewButtons = page.locator('.terminal-area:visible .mobile-controls-row .layout-btn')

    // The pager is all that is left on the row: two windows, two buttons, and
    // no layout counts.
    await expect(viewButtons).toHaveCount(2)
    await expect(viewButtons.first()).toHaveClass(/active/)

    await viewButtons.nth(1).click()
    await expect(viewButtons.nth(1)).toHaveClass(/active/)
    // Still only one visible window
    await expect(page.locator('.terminal-window:visible')).toHaveCount(1)

    await page.click('.hamburger-btn')
    await page.click('.mobile-nav-item:has-text("Terminal tab options")')

    const windowsRow = page.locator('.menu-sheet .menu-row', { hasText: 'Windows' })
    await expect(windowsRow).toContainText('2')
    await windowsRow.click()
    await page.locator('.menu-submenu .menu-row', { hasText: /^3$/ }).click()

    await expect(viewButtons).toHaveCount(3)

    // The menu reads the new count back the next time it is opened.
    await page.click('.hamburger-btn')
    await page.click('.mobile-nav-item:has-text("Terminal tab options")')
    await expect(page.locator('.menu-sheet .menu-row', { hasText: 'Windows' })).toContainText('3')
    await page.keyboard.press('Escape')

    // A nav item switches the tab and shuts the dropdown behind itself.
    await page.click('.hamburger-btn')
    await expect(page.locator('.mobile-nav-dropdown')).toBeVisible()
    await page.click('.mobile-nav-item:has-text("Files")')

    await expect(page.locator('.mobile-nav-dropdown')).toHaveCount(0)
    await expect(page.locator('.files-view')).toBeVisible()
    await expect(page.locator('.mobile-active-tab')).toContainText('Files')
  })
})
