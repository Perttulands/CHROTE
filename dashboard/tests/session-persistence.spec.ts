
import { test, expect, Page } from '@playwright/test'
import { mockApiRoutes } from './mock-api'

// Copied from dashboard.spec.ts
async function dragAndDrop(page: Page, sourceSelector: string, targetSelector: string) {
  const source = page.locator(sourceSelector).first()
  const target = page.locator(targetSelector).first()

  const sourceBox = await source.boundingBox()
  const targetBox = await target.boundingBox()

  if (!sourceBox || !targetBox) {
    throw new Error('Could not find source or target element')
  }

  const startX = sourceBox.x + sourceBox.width / 2
  const startY = sourceBox.y + sourceBox.height / 2
  const endX = targetBox.x + targetBox.width / 2
  const endY = targetBox.y + targetBox.height / 2

  await page.mouse.move(startX, startY)
  await page.mouse.down()
  await page.mouse.move(startX + 10, startY + 10, { steps: 5 })
  await page.mouse.move(endX, endY, { steps: 10 })
  await page.waitForTimeout(100)
  await page.mouse.up()
  await page.waitForTimeout(100)
}

test.describe('Session Persistence', () => {
  test('should persist bound sessions even if API returns empty list', async ({ page }) => {
    // 1. Initial Load with Sessions
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    
    // 2. Drag a session to the first window
    // We target "hq-deacon" from the session panel
    const sessionSource = '.session-item:has-text("hq-deacon")'
    const windowTarget = '.terminal-window' 
    
    // Wait for the session item to be available
    await page.waitForSelector(sessionSource)
    
    // Drag it
    await dragAndDrop(page, sessionSource, windowTarget)
    
    // Verify it is bound (tag appears in window header)
    const sessionTag = page.locator('.terminal-window-header .session-tags .session-tag:has-text("deacon")')
    await expect(sessionTag).toBeVisible()

    // 3. Mock API to return empty list (Simulate server outage)
    await page.route('**/api/tmux/sessions', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          sessions: [],
          grouped: {},
          timestamp: new Date().toISOString()
        }),
      })
    })

    // 4. Force a reload to verify persistence across page loads
    await page.reload()
    await page.waitForSelector('.dashboard')

    // 5. Assert the session tag is STILL THERE
    await expect(sessionTag).toBeVisible({ timeout: 10000 })
  })
})
