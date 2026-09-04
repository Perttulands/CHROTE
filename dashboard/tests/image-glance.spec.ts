import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * The image glance (bead: chrote-5grx.45).
 *
 * An agent prints the path of a screenshot; the path is a link, and the link
 * opens the picture in a glance rather than in Files. Escape with the
 * terminal focused closes it and sends nothing to the pane, a press outside
 * closes it, and a non-image path still opens Files. The Files panel's own
 * picture opens the same glance on a click. Link hit-testing and the image's
 * pixels need a real browser, which is why this is here.
 */

const TTYD_OUTPUT = 0x30
const IMAGE_PATH = '/tmp/shot.png'
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

/** A terminal that prints both paths on one line, and records what is typed at it. */
async function serveTerminal(page: Page) {
  const typed: string[] = []
  const grid = { columns: 0 }
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (text.startsWith('{')) {
        grid.columns = (JSON.parse(text) as { columns: number }).columns
        ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`saved ${IMAGE_PATH} and ${TEXT_PATH}`)]))
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

  test('opens from the picture in the Files panel', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
      // The panel left open on the picture, as the operator left it.
      localStorage.setItem('chrote.workspaceFiles.v1', JSON.stringify({
        version: 1,
        workspaces: {
          terminal1: {
            currentPath: '/tmp',
            expandedPaths: ['/', '/tmp'],
            selectedPath: '/tmp/shot.png',
            openPath: '/tmp/shot.png',
            treeScrollTop: 0,
            viewStates: {},
          },
        },
      }))
    }, seededState())
    await page.goto('/')
    await page.getByRole('button', { name: 'Files sidecar', exact: true }).click()

    const panel = page.locator('.terminal-files-panel')
    const picture = panel.getByRole('button', { name: 'shot.png' })
    await expect(picture).toBeVisible()
    // The panel says the picture's pixels beneath it.
    await expect(panel.locator('.files-panel-note')).toHaveText('3 × 2')

    await picture.click()
    const glance = page.locator('.image-glance')
    await expect(glance).toBeVisible()
    await expect(glance.locator('.image-glance-size')).toHaveText('3 × 2')

    await glance.getByRole('button', { name: 'Close' }).click()
    await expect(glance).toHaveCount(0)
  })
})
