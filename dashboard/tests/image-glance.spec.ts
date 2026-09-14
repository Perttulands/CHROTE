import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * The image glance (bead: chrote-5grx.45).
 *
 * An agent prints the path of a screenshot; the path is a link, and the link
 * opens the picture in a glance rather than in Files. Escape with the
 * terminal focused closes it and sends nothing to the pane, a press outside
 * closes it, and a non-image path still opens Files. The Files panel's own
 * picture opens the same glance on a click, and a corner dragged there is the
 * size every later glance opens at (bead: chrote-dx3r). Link hit-testing, the
 * image's pixels and a real pointer drag through pointer capture need a real
 * browser, which is why these are here.
 */

const TTYD_OUTPUT = 0x30
const IMAGE_PATH = '/tmp/shot.png'
const OTHER_IMAGE_PATH = '/tmp/other.png'
const TEXT_PATH = '/tmp/notes.txt'
/** A 3 by 2 PNG, red. */
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAMAAAACCAIAAAASFvFNAAAAEElEQVR4nGM4IScHQQxwFgBBAAYZPEVBlgAAAABJRU5ErkJggg==', 'base64')

function seededState() {
  return {
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [{ id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 }],
      },
      terminal2: { windowCount: 1, windows: [] },
      terminal3: { windowCount: 1, windows: [] },
    },
    sidebarCollapsed: false,
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
  }
}

/** Files that answer: /tmp lists both files, the PNG is bytes, the text is text. */
async function mockFiles(page: Page) {
  await page.route('**/api/files/raw/**', async route => {
    const png = new URL(route.request().url()).pathname.endsWith('.png')
    await route.fulfill(png
      ? { status: 200, contentType: 'image/png', body: PNG }
      : { status: 200, contentType: 'text/plain', body: 'mock file content' })
  })
  await page.route('**/api/files/diff*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ path: TEXT_PATH, repository: '', diff: '', truncated: false }),
  }))
  await page.route(/\/api\/files\/resources\/?$/, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        isDir: true,
        items: [
          { name: 'shot.png', size: PNG.length, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'image/png' },
        ],
      }),
    })
  })
  await page.route(/\/api\/files\/resources\/tmp\/?$/, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        isDir: true,
        items: [
          { name: 'shot.png', size: PNG.length, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'image/png' },
          { name: 'notes.txt', size: 17, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'text/plain' },
        ],
      }),
    })
  })
}

/** A terminal that prints the given paths on one line, and records what is typed at it. */
async function serveTerminal(page: Page, line = `saved ${IMAGE_PATH} and ${TEXT_PATH}`) {
  const typed: string[] = []
  const grid = { columns: 0 }
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (text.startsWith('{')) {
        grid.columns = (JSON.parse(text) as { columns: number }).columns
        ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(line)]))
      } else if (text.startsWith('0')) {
        typed.push(text.slice(1))
      }
    })
  })
  return { typed, grid }
}

/** The middle of a printed word, in page coordinates. */
async function pointAt(page: Page, columns: number, column: number) {
  const row = page.locator('.terminal-window-body .xterm-rows > div').first()
  const box = (await row.boundingBox())!
  return { x: box.x + (box.width / columns) * column, y: box.y + box.height / 2 }
}

test.describe('the image glance', () => {
  test('opens from an image path in a terminal, closes on Escape and on a press outside, and leaves other paths to Files', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    const { typed, grid } = await serveTerminal(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')
    await expect(page.locator('.terminal-window-body .xterm-rows')).toContainText(IMAGE_PATH)

    // 'saved ' is six cells; the image path runs from there, then ' and '.
    const imagePoint = await pointAt(page, grid.columns, 6 + IMAGE_PATH.length / 2)
    const textPoint = await pointAt(page, grid.columns, 6 + IMAGE_PATH.length + 5 + TEXT_PATH.length / 2)

    await page.mouse.click(imagePoint.x, imagePoint.y)
    const glance = page.locator('.image-glance')
    await expect(glance).toBeVisible()
    await expect(glance.locator('.image-glance-path')).toHaveAttribute('title', IMAGE_PATH)
    await expect(glance.locator('.image-glance-size')).toHaveText('3 × 2')
    // Never upscaled: three pixels wide is three pixels wide.
    await expect.poll(async () => (await glance.locator('img').boundingBox())?.width).toBe(3)

    // The click left the cursor in the terminal; Escape closes the glance and
    // sends nothing to the pane.
    await page.keyboard.press('Escape')
    await expect(glance).toHaveCount(0)

    await page.mouse.click(imagePoint.x, imagePoint.y)
    await expect(glance).toBeVisible()
    const tile = (await page.locator('.terminal-workspace-dock[data-active="true"] .terminal-window-body').boundingBox())!
    await page.mouse.click(tile.x + tile.width - 24, tile.y + tile.height - 24)
    await expect(glance).toHaveCount(0)

    expect(JSON.stringify(typed)).not.toContain('\\u001b')

    // The path beside it is not a picture, so it opens in Files.
    await page.mouse.click(textPoint.x, textPoint.y)
    const panel = page.locator('.terminal-files-panel')
    await expect(panel).toBeVisible()
    await expect(panel.locator('.files-panel-viewer-path')).toHaveAttribute('title', TEXT_PATH)
    await expect(glance).toHaveCount(0)
  })

  /**
   * The Files tab's own viewer is where a picture still opens the centred
   * glance: the terminal workspace reads a picture in the panel's pop-out,
   * and the tab has no panel to hang one off.
   */
  async function openGlanceFromFilesTab(page: Page, alreadyOpen = false) {
    await page.click('.tab:has-text("Files")')
    // A reload brings the tab back on the file the operator left open.
    if (!alreadyOpen) await page.click('.fb-row:has-text("shot.png")')
    await page.getByTestId('file-viewer-scroll').getByRole('button', { name: 'shot.png' }).click()
    const glance = page.locator('.image-glance')
    await expect(glance).toBeVisible()
    return glance
  }

  test('opens from the picture in the Files tab', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')

    const glance = await openGlanceFromFilesTab(page)
    await expect(glance.locator('.image-glance-size')).toHaveText('3 × 2')

    await glance.getByRole('button', { name: 'Close' }).click()
    await expect(glance).toHaveCount(0)
  })

  test('opens at the size a corner was dragged to, and again after a reload', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')

    const openGlance = (alreadyOpen = false) => openGlanceFromFilesTab(page, alreadyOpen)

    const glance = await openGlance()
    // The picture is three pixels wide, so the glance opens tiny: its own
    // size, not a minimum.
    const opened = (await glance.boundingBox())!
    expect(opened.width).toBeLessThan(100)

    // Drag the bottom-right corner out. The window is centred, so the corner
    // follows the pointer while the window grows around its middle.
    await page.mouse.move(opened.x + opened.width - 2, opened.y + opened.height - 2)
    await page.mouse.down()
    await page.mouse.move(opened.x + opened.width + 200, opened.y + opened.height + 150, { steps: 10 })
    await page.mouse.up()

    const dragged = (await glance.boundingBox())!
    expect(dragged.width).toBeGreaterThan(opened.width + 300)
    expect(dragged.height).toBeGreaterThan(opened.height + 200)

    await page.reload()
    const reopened = await openGlance(true)
    const after = (await reopened.boundingBox())!
    expect(Math.round(after.width)).toBe(Math.round(dragged.width))
    expect(Math.round(after.height)).toBe(Math.round(dragged.height))

    // The word in the header gives the picture the say back.
    await reopened.getByRole('button', { name: 'Reset size' }).click()
    await expect.poll(async () => (await reopened.boundingBox())!.width).toBeLessThan(100)
  })

  /**
   * The level is the operator's, not the picture's (bead: chrote-4689): a step
   * taken on one picture is the level the next picture opens at. The key has
   * to reach the model past a focused terminal, which is browser-only.
   */
  test('carries the stepped zoom level to the next picture', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    const { grid } = await serveTerminal(page, `saved ${IMAGE_PATH} and ${OTHER_IMAGE_PATH}`)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')
    await expect(page.locator('.terminal-window-body .xterm-rows')).toContainText(IMAGE_PATH)

    const firstPoint = await pointAt(page, grid.columns, 6 + IMAGE_PATH.length / 2)
    const secondPoint = await pointAt(page, grid.columns, 6 + IMAGE_PATH.length + 5 + OTHER_IMAGE_PATH.length / 2)

    await page.mouse.click(firstPoint.x, firstPoint.y)
    const glance = page.locator('.image-glance')
    await expect(glance).toBeVisible()
    // Three pixels wide inside a window that fits it: drawn at 1:1.
    await expect(glance.locator('.image-glance-zoom')).toHaveText('100%')

    await page.keyboard.press('Alt+Equal')
    await expect(glance.locator('.image-glance-zoom')).toHaveText('125%')
    await expect.poll(async () => (await glance.locator('img').boundingBox())?.width).toBe(4)

    await page.keyboard.press('Escape')
    await expect(glance).toHaveCount(0)

    await page.mouse.click(secondPoint.x, secondPoint.y)
    await expect(glance).toBeVisible()
    await expect(glance.locator('.image-glance-path')).toHaveAttribute('title', OTHER_IMAGE_PATH)
    await expect(glance.locator('.image-glance-zoom')).toHaveText('125%')
    await expect.poll(async () => (await glance.locator('img').boundingBox())?.width).toBe(4)
  })
})
