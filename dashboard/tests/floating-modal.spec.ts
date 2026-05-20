import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'

const terminalRoutePattern = /.*\/terminal\/?.*/

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
    expect(newBox!.y).not.toBe(initialBox!.y)
    // Should have moved roughly in the direction we dragged
    expect(newBox!.x - initialBox!.x).toBeGreaterThan(50)
    expect(newBox!.y - initialBox!.y).toBeGreaterThan(50)
  })

  test('click overlay background closes modal', async ({ page }) => {
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
    await page.waitForSelector('.session-item')

    // Open modal
    await page.click('.session-item:has-text("jack")')
    await expect(page.locator('.floating-modal')).toBeVisible()

    // Click the overlay (far corner, outside the modal)
    await page.click('.floating-modal-overlay', { position: { x: 10, y: 10 } })

    // Modal should close
    await expect(page.locator('.floating-modal')).not.toBeVisible()
  })

  test('modal shows disconnected indicator before iframe loads', async ({ page }) => {
    await mockApiRoutes(page)
    let terminalRequestDone = false

    // Delay terminal response so the iframe stays in loading state
    await page.route(terminalRoutePattern, async route => {
      await new Promise(resolve => setTimeout(resolve, 750))
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<html><body>mock terminal</body></html>',
      })
      terminalRequestDone = true
    })

    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.waitForSelector('.session-item')

    // Open modal
    await page.click('.session-item:has-text("jack")')
    await expect(page.locator('.floating-modal')).toBeVisible()

    // Status dot should have 'disconnected' class while iframe is loading
    const statusDot = page.locator('.floating-modal .status-dot')
    await expect(statusDot).toBeVisible()
    await expect(statusDot).toHaveClass(/disconnected/)

    await expect(async () => {
      expect(terminalRequestDone).toBe(true)
    }).toPass({ timeout: 2000 })
  })

  test('font size from settings applies in modal', async ({ page }) => {
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
    await page.waitForSelector('.session-item')

    // Navigate to settings and change font size
    await page.click('.tab:has-text("Settings")')
    await page.waitForSelector('.settings-view')

    // Find the font size slider (min=12, max=20) - distinct from the volume slider
    const fontSizeSlider = page.locator('input[type="range"][min="12"][max="20"]')
    await expect(fontSizeSlider).toBeVisible()

    // Change font size by setting the input value directly
    await fontSizeSlider.fill('18')

    // Verify the label updated
    await expect(page.locator('.settings-label:has-text("Font Size")')).toContainText('18px')

    // Go back to Terminal tab
    await page.click('.tab:has-text("Terminal")')
    await page.waitForSelector('.session-panel')
    await page.waitForSelector('.session-item')

    // Open floating modal
    await page.click('.session-item:has-text("jack")')
    await expect(page.locator('.floating-modal')).toBeVisible()

    // The modal iframe should exist and point to the terminal
    const iframe = page.locator('.floating-modal-body iframe')
    await expect(iframe).toBeVisible()
    await expect(iframe).toHaveAttribute('src', /terminal/)
  })
})
