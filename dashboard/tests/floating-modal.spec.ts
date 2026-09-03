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
  await openSessionsSidecar(page)
  await page.waitForSelector('.session-item')
  await page.click('.session-item:has-text("jack")')
  await expect(page.locator('.sheet.sheet-left')).toBeVisible()
}

test.describe('Peek (chrote-5grx.22)', () => {
  test('docks at the left, snapped to a tile boundary, with the tiles beside it still readable', async ({ page }) => {
    await openPeek(page)

    const peek = page.locator('.sheet.sheet-left')
    const peekBox = await peek.boundingBox()
    expect(peekBox).toBeTruthy()

    const workspace = page.locator('.dashboard-content')
    const workspaceBox = await workspace.boundingBox()
    expect(workspaceBox).toBeTruthy()

    // Never more than 60% of the workspace, and left-docked.
    expect(peekBox!.x).toBeLessThanOrEqual(workspaceBox!.x + 1)
    expect(peekBox!.width).toBeLessThanOrEqual(workspaceBox!.width * 0.6 + 1)

    // The width lands on a tile's own edge, so no tile is cut mid-glyph.
    const tileEdges = await page.locator('.terminal-workspace-dock[data-active="true"] .terminal-window')
      .evaluateAll(tiles => tiles.map(tile => tile.getBoundingClientRect().right))
    const snapped = tileEdges.some(edge => Math.abs(edge - (peekBox!.x + peekBox!.width)) <= 1)
    expect(snapped).toBe(true)

    // The status line is a full-width footer, so a left sheet covers the panel
    // without truncating the line.
    const statusBox = await page.locator('.status-line').boundingBox()
    expect(statusBox).toBeTruthy()
    expect(statusBox!.x).toBeLessThanOrEqual(1)
    expect(statusBox!.y).toBeGreaterThanOrEqual(peekBox!.y + peekBox!.height - 1)
  })

  test('lays no backdrop, so a selection released outside it keeps both the selection and the sheet', async ({ page }) => {
    await openPeek(page)
    const rows = page.locator('.sheet-left .xterm-rows')
    await expect(rows).toContainText(PEEK_LINE)

    const row = page.locator('.sheet-left .xterm-rows > div').first()
    const box = await row.boundingBox()
    expect(box).toBeTruthy()
    const y = box!.y + box!.height / 2
    await page.keyboard.down('Shift')
    await page.mouse.move(box!.x + box!.width / 2, y)
    await page.mouse.down()
    await page.mouse.move(box!.x + 1, y, { steps: 5 })
    await page.mouse.move(4, y, { steps: 5 })
    await page.mouse.up()
    await page.keyboard.up('Shift')

    await expect(page.locator('.sheet.sheet-left')).toBeVisible()
    await expect(page.locator('.sheet-left .xterm-selection > div')).not.toHaveCount(0)
  })

  test('closes on Escape and from its header, and not from a click outside', async ({ page }) => {
    await openPeek(page)

    // A click on a tile beside Peek is a click on that tile, nothing more.
    await page.locator('.terminal-workspace-dock[data-active="true"] .terminal-window').last().click({ position: { x: 8, y: 8 } })
    await expect(page.locator('.sheet.sheet-left')).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.locator('.sheet.sheet-left')).toHaveCount(0)

    await page.click('.session-item:has-text("jack")')
    await expect(page.locator('.sheet.sheet-left')).toBeVisible()
    await page.getByRole('button', { name: 'Close Peek' }).click()
    await expect(page.locator('.sheet.sheet-left')).toHaveCount(0)
  })
})
