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

  test('hamburger button is visible at mobile viewport', async ({ page }) => {
    const hamburger = page.locator('.hamburger-btn')
    await expect(hamburger).toBeVisible()
  })

  test('tab-bar has mobile-mode class', async ({ page }) => {
    await expect(page.locator('.tab-bar')).toHaveClass(/mobile-mode/)
  })

  test('desktop tab row is not visible', async ({ page }) => {
    await expect(page.locator('.tab-bar-tabs')).toHaveCount(0)
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
    await expect(dropdown.locator('.mobile-nav-item', { hasText: /^Agents$/ })).toBeVisible()
    await expect(dropdown.locator('.mobile-nav-item', { hasText: /^Settings$/ })).toBeVisible()
  })

  test('clicking hamburger again closes dropdown', async ({ page }) => {
    await page.click('.hamburger-btn')
    await expect(page.locator('.mobile-nav-dropdown')).toBeVisible()

    await page.click('.hamburger-btn')
    await expect(page.locator('.mobile-nav-dropdown')).toHaveCount(0)
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

  test('active tab is highlighted in dropdown', async ({ page }) => {
    // Default tab is terminal1
    await page.click('.hamburger-btn')

    // Use exact match for "Terminal" (not "Terminal 2")
    const terminalItem = page.locator('.mobile-nav-item', { hasText: /^Terminal$/ })
    await expect(terminalItem).toHaveClass(/active/)
  })

  test('switching tabs updates active label in tab bar', async ({ page }) => {
    // Default label
    await expect(page.locator('.mobile-active-tab')).toContainText('Terminal')

    // Switch to Settings
    await page.click('.hamburger-btn')
    await page.click('.mobile-nav-item:has-text("Settings")')

    await expect(page.locator('.mobile-active-tab')).toContainText('Settings')
  })
})

test.describe('Terminal Area - Mobile View Switcher', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test('shows mobile per-window view switcher', async ({ page }) => {
    // Scope to visible terminal area (only the active workspace is visible)
    const visibleArea = page.locator('.terminal-area:visible')

    // Mobile controls row should be visible
    await expect(visibleArea.locator('.mobile-controls-row')).toBeVisible()

    // "View:" label should be present
    await expect(visibleArea.locator('.layout-label:has-text("View")')).toBeVisible()
  })

  test('only one terminal window is visible at a time', async ({ page }) => {
    // With default 2-window layout, only 1 should be visible on mobile
    const visibleWindows = page.locator('.terminal-window:visible')
    await expect(visibleWindows).toHaveCount(1)
  })

  test('view buttons switch between windows', async ({ page }) => {
    // Scope to the visible terminal area's controls
    const controlsRow = page.locator('.terminal-area:visible .mobile-controls-row')
    const viewButtons = controlsRow.locator('.layout-btn')

    // Default 2 windows means view buttons 1 and 2, plus 4 count buttons.
    // Button "1" (first view button) should be active by default
    await expect(viewButtons.first()).toHaveClass(/active/)

    // Click button "2" to switch to second window
    await viewButtons.nth(1).click()
    await expect(viewButtons.nth(1)).toHaveClass(/active/)

    // Still only one visible window
    await expect(page.locator('.terminal-window:visible')).toHaveCount(1)
  })

  test('mobile view buttons 1-4 work with 4-window layout', async ({ page }) => {
    const controlsRow = page.locator('.terminal-area:visible .mobile-controls-row')
    const allButtons = controlsRow.locator('.layout-btn')

    // With default 2 windows: [View:1] [View:2] | [Count:1] [Count:2] [Count:3] [Count:4]
    await expect(allButtons).toHaveCount(6)
    const count4Btn = controlsRow.locator('.layout-btn', { hasText: /^4$/ })
    await count4Btn.click()

    // Now we should have 4 view buttons + 4 count buttons = 8 buttons
    await expect(allButtons).toHaveCount(8)

    // Still only 1 visible window
    await expect(page.locator('.terminal-window:visible')).toHaveCount(1)

    // Click view button 3 (index 2 among the first 4)
    await allButtons.nth(2).click()
    await expect(allButtons.nth(2)).toHaveClass(/active/)

    // Click view button 4
    await allButtons.nth(3).click()
    await expect(allButtons.nth(3)).toHaveClass(/active/)

    // Still only 1 visible
    await expect(page.locator('.terminal-window:visible')).toHaveCount(1)
  })

  test('layout count buttons work on mobile', async ({ page }) => {
    const controlsRow = page.locator('.terminal-area:visible .mobile-controls-row')
    const allButtons = controlsRow.locator('.layout-btn')

    // Default 2 windows: [View:1] [View:2] | [Count:1] [Count:2] [Count:3] [Count:4]
    // 6 buttons total. Count buttons start at index 2.
    // Count "2" (index 3) should be active since default is 2 windows
    await expect(allButtons).toHaveCount(6)
    await expect(allButtons.nth(3)).toHaveClass(/active/)

    // Switch to 1 window: click Count "1" (index 2)
    await allButtons.nth(2).click()

    // Now only 1 view button + 4 count buttons = 5 buttons
    await expect(allButtons).toHaveCount(5)

    // Switch to 3 windows: Count "3" is now at index 3
    await allButtons.nth(3).click()

    // 3 view buttons + 4 count buttons = 7
    await expect(allButtons).toHaveCount(7)
  })

  test('grid uses grid-1 class on mobile regardless of window count', async ({ page }) => {
    // Even with default 2 windows, mobile should use grid-1
    await expect(page.locator('.terminal-grid:visible')).toHaveClass(/grid-1/)
  })
})

test.describe('Responsive Breakpoint', () => {
  test('at 769px wide, desktop layout shows', async ({ page }) => {
    await page.setViewportSize({ width: 769, height: 844 })
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    // Desktop: tab-bar-tabs should exist, no hamburger
    await expect(page.locator('.tab-bar-tabs')).toBeVisible()
    await expect(page.locator('.hamburger-btn')).toHaveCount(0)
    await expect(page.locator('.tab-bar')).not.toHaveClass(/mobile-mode/)
  })

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
