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

  test('fills the panel with equal-height instrument rows and full-width traces', async ({ page }) => {
    await mockSystemStatusApiRoutes(page)

    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Server")')
    await expect(page.locator('.system-instrument').first()).toBeVisible()

    const board = await page.locator('.system-instruments').boundingBox()
    const rows = await page.locator('.system-instrument').all()
    const boxes = await Promise.all(rows.map(row => row.boundingBox()))
    const heights = boxes.map(box => box?.height ?? 0)

    // Rows share the panel evenly instead of sitting at a fixed height with dead space below.
    expect(Math.max(...heights) - Math.min(...heights)).toBeLessThan(2)
    const covered = heights.reduce((total, height) => total + height, 0)
    expect(covered).toBeGreaterThan((board?.height ?? 0) * 0.9)

    // The trace stretches to the chart column rather than a hard-coded pixel width.
    const chart = await page.locator('.system-instrument-chart').first().boundingBox()
    const trace = await page.locator('.system-trace').first().boundingBox()
    expect(trace?.width ?? 0).toBeGreaterThan((chart?.width ?? 0) * 0.9)
    await expect(page.locator('.system-timeline-scroll')).toHaveCount(0)
  })

  test('uses one theme-aware trace color for every instrument', async ({ page }) => {
    await mockSystemStatusApiRoutes(page)
    await page.addInitScript(() => {
      localStorage.clear()
    })

    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Server")')

    const memoryTrace = page.locator('.system-instrument-memory .system-trace-line, .system-instrument-memory .system-trace-stem').first()
    const loadTrace = page.locator('.system-instrument-load .system-trace-line, .system-instrument-load .system-trace-stem').first()
    await expect(memoryTrace).toBeVisible()
    await expect(loadTrace).toBeVisible()

    const darkStroke = await memoryTrace.evaluate((node) => getComputedStyle(node).stroke)
    const darkLoadStroke = await loadTrace.evaluate((node) => getComputedStyle(node).stroke)
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'matrix'))
    const matrixStroke = await memoryTrace.evaluate((node) => getComputedStyle(node).stroke)

    expect(darkStroke).toBeTruthy()
    expect(darkLoadStroke).toBe(darkStroke)
    expect(matrixStroke).toBeTruthy()
    expect(darkStroke).not.toBe(matrixStroke)
    await expect(page.locator('.system-tui-bar')).toHaveCount(0)
    await expect(page.locator('.system-donut')).toHaveCount(0)
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
