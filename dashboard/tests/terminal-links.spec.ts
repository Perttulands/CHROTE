import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Terminal link gate (bead: chrote-wgqp.4).
 *
 * Agent output is full of URLs the operator has to open — pull requests, CI
 * runs, the links agents are told to report. ttyd's client loaded the web-links
 * addon, so they were clickable until CHROTE took the terminal over.
 *
 * The URL is never actually fetched here: window.open is recorded instead, so
 * the test says what the click asked for without leaving the host.
 */

const TTYD_OUTPUT = 0x30
const PRINTED_URL = 'https://example.com/deep/link'

declare global {
  interface Window {
    __openedUrls?: string[]
  }
}

function seededState() {
  return {
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 },
        ],
      },
      terminal2: { windowCount: 1, windows: [] },
      terminal3: { windowCount: 1, windows: [] },
    },
    sidebarCollapsed: false,
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
  }
}

async function openTerminalWithUrl(page: Page) {
  await mockApiRoutes(page)
  const grid = { columns: 0 }
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (!text.startsWith('{')) return
      grid.columns = (JSON.parse(text) as { columns: number }).columns
      ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`see ${PRINTED_URL} for the run`)]))
    })
  })
  // The addon opens a blank tab and then points it at the URL, so the stub has
  // to be a tab-shaped object rather than a recorder of window.open arguments.
  await page.addInitScript(() => {
    window.__openedUrls = []
    window.open = () => ({
      opener: null,
      location: { set href(value: string) { window.__openedUrls?.push(value) } },
    } as unknown as Window)
  })
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState())
  await page.goto('/')
  const rows = page.locator('.terminal-window-body .xterm-rows')
  await expect(rows).toContainText(PRINTED_URL)
  return grid
}

/** The middle of the printed URL, in page coordinates. */
async function urlPoint(page: Page, columns: number) {
  const row = page.locator('.terminal-window-body .xterm-rows > div').first()
  const box = await row.boundingBox()
  if (!box) throw new Error('no first terminal row')
  const cell = box.width / columns
  // 'see ' is four cells, and the URL runs from there.
  const column = 4 + PRINTED_URL.length / 2
  return { x: box.x + cell * column, y: box.y + box.height / 2 }
}

const PRINTED_PATH = '/tmp/notes.txt'
const PRINTED_PICTURE = '/tmp/shot.png'
/** A 3 by 2 PNG, red. */
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAMAAAACCAIAAAASFvFNAAAAEElEQVR4nGM4IScHQQxwFgBBAAYZPEVBlgAAAABJRU5ErkJggg==', 'base64')

/**
 * A terminal that prints one absolute path, and a Files API that lists its
 * parent and serves its bytes (bead: chrote-wgqp.7).
 */
async function openTerminalWithPath(page: Page, printed = PRINTED_PATH) {
  await mockApiRoutes(page)
  await page.route('**/api/files/raw/**', route => route.fulfill(
    new URL(route.request().url()).pathname.endsWith('.png')
      ? { status: 200, contentType: 'image/png', body: PNG }
      : { status: 200, contentType: 'text/plain', body: 'mock file content' },
  ))
  // The viewer asks once whether the file sits in a repository.
  await page.route('**/api/files/diff*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ path: PRINTED_PATH, repository: '', diff: '', truncated: false }),
  }))
  await page.route(/\/api\/files\/resources\/tmp\/?$/, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        isDir: true,
        items: [
          { name: 'notes.txt', size: 17, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'text/plain' },
          { name: 'shot.png', size: PNG.length, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'image/png' },
        ],
      }),
    })
  })
  const grid = { columns: 0 }
  const typed: string[] = []
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (text.startsWith('0')) {
        typed.push(text.slice(1))
        return
      }
      if (!text.startsWith('{')) return
      grid.columns = (JSON.parse(text) as { columns: number }).columns
      ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`see ${printed} for the run`)]))
    })
  })
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState())
  await page.goto('/')
  await expect(page.locator('.terminal-window-body .xterm-rows')).toContainText(printed)
  return { grid, typed }
}

/** The middle of the printed path, in page coordinates. */
async function pathPoint(page: Page, columns: number, printed: string) {
  const row = page.locator('.terminal-window-body .xterm-rows > div').first()
  const box = (await row.boundingBox())!
  const cell = box.width / columns
  // 'see ' is four cells, and the path runs from there.
  return { x: box.x + cell * (4 + printed.length / 2), y: box.y + box.height / 2 }
}

test.describe('Terminal links', () => {
  test('a printed URL is hoverable and opens in a new tab', async ({ page }) => {
    const grid = await openTerminalWithUrl(page)
    const point = await urlPoint(page, grid.columns)

    await page.mouse.move(point.x, point.y)
    await expect(page.locator('.terminal-window-body .xterm-screen.xterm-cursor-pointer')).toBeVisible()

    await page.mouse.click(point.x, point.y)

    await expect.poll(() => page.evaluate(() => window.__openedUrls)).toEqual([PRINTED_URL])
  })

  test('a printed absolute path opens the file in the Files panel', async ({ page }) => {
    const { grid } = await openTerminalWithPath(page)
    const point = await pathPoint(page, grid.columns, PRINTED_PATH)

    await page.mouse.move(point.x, point.y)
    await expect(page.locator('.terminal-window-body .xterm-screen.xterm-cursor-pointer')).toBeVisible()
    await page.mouse.click(point.x, point.y)

    const panel = page.locator('.terminal-files-panel')
    await expect(panel).toBeVisible()
    await expect(panel.locator('.files-panel-viewer-path')).toHaveAttribute('title', PRINTED_PATH)
    await expect(panel.locator('[data-ui="files.viewer"]')).toContainText('mock file content')
  })

  // A picture takes the same way in as any other path now: the panel opens,
  // walks to the parent, selects the row and pops the picture out beside the
  // tree. Link hit-testing on real cell geometry is why this is a journey.
  test('a printed picture opens the Files panel and pops the picture out', async ({ page }) => {
    const { grid, typed } = await openTerminalWithPath(page, PRINTED_PICTURE)
    const point = await pathPoint(page, grid.columns, PRINTED_PICTURE)

    await page.mouse.click(point.x, point.y)

    const panel = page.locator('.terminal-files-panel')
    await expect(panel).toBeVisible()
    await expect(panel.getByRole('treeitem', { name: /shot\.png/ })).toHaveAttribute('aria-selected', 'true')

    const popout = page.getByRole('dialog', { name: 'File shot.png' })
    await expect(popout).toBeVisible()
    await expect(popout.getByRole('img', { name: 'shot.png' })).toBeVisible()
    await expect(popout.locator('.files-panel-note')).toHaveText('3 × 2')

    // The click left the cursor in the terminal: Escape closes the pop-out,
    // leaves the tree, and sends nothing to the pane.
    await page.keyboard.press('Escape')
    await expect(popout).toHaveCount(0)
    await expect(panel.getByRole('tree', { name: 'File tree' })).toBeVisible()
    expect(JSON.stringify(typed)).not.toContain('\\u001b')
  })
})
