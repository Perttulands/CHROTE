import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { openSessionsSidecar } from './helpers'

const TTYD_OUTPUT = 0x30
const MOUSE_MODE_ON = '\u001b[?1000h\u001b[?1002h\u001b[?1006h'
const PEEK_LINE = 'PEEK-SELECT-ME'

/** Answer the peek handshake with tmux's mouse mode and one line to paint. */
async function servePeekTerminal(page: Page) {
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (!text.startsWith('{')) return
      ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`${MOUSE_MODE_ON}${PEEK_LINE}`)]))
    })
  })
}

async function openPeek(page: Page) {
  await mockApiRoutes(page)
  await servePeekTerminal(page)
  await page.goto('/')
  await page.waitForSelector('.dashboard')
  await openSessionsSidecar(page, { pin: false })
  await page.waitForSelector('.session-item')
  await page.click('.session-item:has-text("jack")')
  await expect(page.locator('.floating-modal')).toBeVisible()
}

test.describe('Floating Modal (pol-9a4a)', () => {
  test('drag modal header to reposition', async ({ page }) => {
    await mockApiRoutes(page)
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

  // Bead chrote-wgqp.3: a click's target is the common ancestor of its press
  // and its release, so a selection drag released past the modal edge used to
  // dismiss Peek and the selection with it.
  test('a selection drag released outside the modal leaves Peek open', async ({ page }) => {
    await openPeek(page)
    const rows = page.locator('.floating-modal .xterm-rows')
    await expect(rows).toContainText(PEEK_LINE)

    const row = page.locator('.floating-modal .xterm-rows > div').first()
    const box = await row.boundingBox()
    expect(box).toBeTruthy()
    const y = box!.y + box!.height / 2
    await page.keyboard.down('Shift')
    // Paint leftwards from mid-row, so the release lands past the modal edge
    // with a selection still behind it.
    await page.mouse.move(box!.x + box!.width / 2, y)
    await page.mouse.down()
    await page.mouse.move(box!.x + 1, y, { steps: 5 })
    // Past the left edge of the modal, onto the overlay.
    await page.mouse.move(4, y, { steps: 5 })
    await page.mouse.up()
    await page.keyboard.up('Shift')

    await expect(page.locator('.floating-modal')).toBeVisible()
    await expect(page.locator('.floating-modal .xterm-selection > div')).not.toHaveCount(0)
  })

  test('a genuine click on the overlay still closes Peek', async ({ page }) => {
    await openPeek(page)

    await page.mouse.click(4, 4)

    await expect(page.locator('.floating-modal')).toHaveCount(0)
  })

})
