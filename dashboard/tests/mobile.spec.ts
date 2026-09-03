import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'

// Mobile viewport applied to all tests in this file via the mobile project config.
// The 'mobile' project in playwright.config.ts sets iPhone 13 device emulation.
// For the responsive breakpoint tests at the bottom, we override the viewport per-test.

test.describe('Hamburger Menu', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test('clicking hamburger opens mobile-nav-dropdown with all tabs', async ({ page }) => {
    // Dropdown should not exist yet
    await expect(page.locator('.mobile-nav-dropdown')).toHaveCount(0)

    // Open hamburger menu
    await page.click('.hamburger-btn')

    // Dropdown should now be visible
    const dropdown = page.locator('.mobile-nav-dropdown')
    await expect(dropdown).toBeVisible()

    // All main nav items should be present (use exact text to avoid ambiguity)
    await expect(dropdown.locator('.mobile-nav-item', { hasText: /^Terminal$/ })).toBeVisible()
    await expect(dropdown.locator('.mobile-nav-item', { hasText: /^Terminal 2$/ })).toBeVisible()
    await expect(dropdown.locator('.mobile-nav-item', { hasText: /^Files$/ })).toBeVisible()
    await expect(dropdown.locator('.mobile-nav-item', { hasText: /^Beads$/ })).toBeVisible()
    await expect(dropdown.locator('.mobile-nav-item', { hasText: /^Settings$/ })).toBeVisible()
  })

  test('clicking a nav item switches tab and closes dropdown', async ({ page }) => {
    await page.click('.hamburger-btn')
    await expect(page.locator('.mobile-nav-dropdown')).toBeVisible()

    // Click Files tab
    await page.click('.mobile-nav-item:has-text("Files")')

    // Dropdown should close
    await expect(page.locator('.mobile-nav-dropdown')).toHaveCount(0)

    // Files view should be visible
    await expect(page.locator('.files-view')).toBeVisible()

    // Active tab label in the bar should update
    await expect(page.locator('.mobile-active-tab')).toContainText('Files')
  })

})

test.describe('Terminal Area - Mobile View Switcher', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test('view buttons switch between windows', async ({ page }) => {
    // Scope to the visible terminal area's controls
    const controlsRow = page.locator('.terminal-area:visible .mobile-controls-row')
    const viewButtons = controlsRow.locator('.layout-btn')

    // The pager is all that is left on the row: two windows, two buttons, and
    // no layout counts — a phone has no keyboard to reach the chords with.
    await expect(viewButtons).toHaveCount(2)
    await expect(viewButtons.first()).toHaveClass(/active/)

    // Click button "2" to switch to second window
    await viewButtons.nth(1).click()
    await expect(viewButtons.nth(1)).toHaveClass(/active/)

    // Still only one visible window
    await expect(page.locator('.terminal-window:visible')).toHaveCount(1)
  })

  // The count has chords on a desktop and nothing on a phone, so the tab menu
  // carries it — reached here through the hamburger, which is the only way in.
  test('changes the window count through the tab menu in the hamburger', async ({ page }) => {
    const viewButtons = page.locator('.terminal-area:visible .mobile-controls-row .layout-btn')
    await expect(viewButtons).toHaveCount(2)

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
  })

})

test.describe('Responsive Breakpoint', () => {

  test('at 768px wide, mobile layout shows', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 844 })
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    // Mobile: hamburger visible, no tab-bar-tabs
    await expect(page.locator('.hamburger-btn')).toBeVisible()
    await expect(page.locator('.tab-bar-tabs')).toHaveCount(0)
    await expect(page.locator('.tab-bar')).toHaveClass(/mobile-mode/)
  })
})
