import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { openSessionsSidecar } from './helpers'

const terminalRoutePattern = /\/terminal(\/|\?|$)/

test.describe('Floating Modal (pol-9a4a)', () => {
  test('drag modal header to reposition', async ({ page }) => {
    await mockApiRoutes(page)
    await page.route(terminalRoutePattern, async route => {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<html><body>mock terminal</body></html>',
      })
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page, { pin: false })
    await page.waitForSelector('.session-item')

    // Open modal by clicking an unassigned session
    await page.click('.session-item:has-text("jack")')
    await expect(page.locator('.floating-modal')).toBeVisible()

    const modal = page.locator('.floating-modal')
    const header = page.locator('.floating-modal-header')

    // Get initial position
    const initialBox = await modal.boundingBox()
    expect(initialBox).toBeTruthy()

    // Drag the header
    const headerBox = await header.boundingBox()
    expect(headerBox).toBeTruthy()

    const startX = headerBox!.x + headerBox!.width / 2
    const startY = headerBox!.y + headerBox!.height / 2

    await page.mouse.move(startX, startY)
    await page.mouse.down()
    await page.mouse.move(startX + 150, startY + 100, { steps: 10 })
    await page.mouse.up()

    // Verify position changed
    const newBox = await modal.boundingBox()
    expect(newBox).toBeTruthy()
    expect(newBox!.x).not.toBe(initialBox!.x)
    // Horizontal drag moves; vertical movement is clamped to the viewport.
    expect(newBox!.x - initialBox!.x).toBeGreaterThan(20)
    expect(newBox!.y).toBeGreaterThanOrEqual(16)
    expect(newBox!.y + newBox!.height).toBeLessThanOrEqual(await page.evaluate(() => window.innerHeight - 16))
  })

})
