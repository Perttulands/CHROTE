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
    await expect(page.locator('.system-graph-panel')).toContainText(/samples/)
  })

  test('uses theme-aware graph line colors', async ({ page }) => {
    await mockSystemStatusApiRoutes(page)
    await page.addInitScript(() => {
      localStorage.clear()
    })

    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Server")')

    const memoryLine = page.locator('.system-graph-line-memory').first()
    await expect(memoryLine).toBeVisible()

    const darkStroke = await memoryLine.evaluate((node) => getComputedStyle(node).stroke)
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'matrix'))
    const matrixStroke = await memoryLine.evaluate((node) => getComputedStyle(node).stroke)

    expect(darkStroke).toBeTruthy()
    expect(matrixStroke).toBeTruthy()
    expect(darkStroke).not.toBe(matrixStroke)
  })
})
