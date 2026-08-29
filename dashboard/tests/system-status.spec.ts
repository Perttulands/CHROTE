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
    await expect(page.locator('.system-axis-gutter')).toContainText(/samples?/)
    await expect(page.getByText('free 2.0 GB · available 8.0 GB')).toBeVisible()
    await expect(page.locator('.system-instrument')).toHaveCount(6)
    await expect(page.getByText(/TUI-style separated graphs/)).toHaveCount(0)
    await expect(page.getByText(/backend history/)).toHaveCount(0)
  })

  test('scrubs all rows to one hovered moment', async ({ page }) => {
    await mockSystemStatusApiRoutes(page)

    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Server")')
    await expect(page.getByRole('img', { name: 'GPU history' })).toBeVisible()

    await page.getByRole('img', { name: 'GPU history' }).hover({ position: { x: 450, y: 20 } })
    await expect(page.getByLabel('History sample')).toBeVisible()
    await expect(page.getByLabel('History sample')).toContainText(/at\s+\d/)
    await expect(page.locator('.system-trace-crosshair')).toHaveCount(6)
    await expect(page.locator('.system-instrument-reading.is-scrubbed')).toHaveCount(6)
  })
})
