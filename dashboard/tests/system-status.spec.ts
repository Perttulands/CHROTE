import { test, expect } from './fixtures'
import { mockApiRoutes, mockSystemStatusApiRoutes } from './mock-api'

test.describe('System Status View', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
  })

  test('keeps status history warm before the Server tab is active', async ({ page }) => {
    let requests = 0
    await mockSystemStatusApiRoutes(page, () => {
      requests += 1
    })

    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await expect.poll(() => requests, { timeout: 3000 }).toBeGreaterThan(0)
    await expect(page.locator('.system-status-view')).toBeHidden()

    await page.click('.tab:has-text("Server")')
    await expect(page.locator('.system-status-view')).toBeVisible()
    await expect(page.locator('.system-timeline-panel')).toContainText(/samples/)
  })

  test('uses one theme-aware bar color for telemetry strips', async ({ page }) => {
    await mockSystemStatusApiRoutes(page)
    await page.addInitScript(() => {
      localStorage.clear()
    })

    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Server")')

    const memoryBar = page.locator('.system-tui-bar-memory').first()
    const loadBar = page.locator('.system-tui-bar-load').first()
    await expect(memoryBar).toBeVisible()
    await expect(loadBar).toBeVisible()

    const darkFill = await memoryBar.evaluate((node) => getComputedStyle(node).fill)
    const darkLoadFill = await loadBar.evaluate((node) => getComputedStyle(node).fill)
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'matrix'))
    const matrixFill = await memoryBar.evaluate((node) => getComputedStyle(node).fill)

    expect(darkFill).toBeTruthy()
    expect(darkLoadFill).toBe(darkFill)
    expect(matrixFill).toBeTruthy()
    expect(darkFill).not.toBe(matrixFill)
    await expect(page.locator('.system-tui-path')).toHaveCount(0)
    await expect(page.locator('.system-tui-dot')).toHaveCount(0)
  })
})
