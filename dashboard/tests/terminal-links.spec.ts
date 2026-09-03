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

/**
 * A terminal that prints one absolute path, and a Files API that lists its
 * parent and serves its bytes (bead: chrote-wgqp.7).
 */
async function openTerminalWithPath(page: Page) {
  await mockApiRoutes(page)
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
        items: [{ name: 'notes.txt', size: 17, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'text/plain' }],
      }),
    })
  })
  const grid = { columns: 0 }
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (!text.startsWith('{')) return
      grid.columns = (JSON.parse(text) as { columns: number }).columns
      ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`see ${PRINTED_PATH} for the run`)]))
    })
  })
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState())
  await page.goto('/')
  await expect(page.locator('.terminal-window-body .xterm-rows')).toContainText(PRINTED_PATH)
  return grid
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
    const grid = await openTerminalWithPath(page)
    const row = page.locator('.terminal-window-body .xterm-rows > div').first()
    const box = (await row.boundingBox())!
    const cell = box.width / grid.columns
    // 'see ' is four cells, and the path runs from there.
    const point = { x: box.x + cell * (4 + PRINTED_PATH.length / 2), y: box.y + box.height / 2 }

    await page.mouse.move(point.x, point.y)
    await expect(page.locator('.terminal-window-body .xterm-screen.xterm-cursor-pointer')).toBeVisible()
    await page.mouse.click(point.x, point.y)

    const panel = page.locator('.terminal-files-panel')
    await expect(panel).toBeVisible()
    await expect(panel.locator('.files-panel-viewer-path')).toHaveAttribute('title', PRINTED_PATH)
    await expect(panel.locator('[data-ui="files.viewer"]')).toContainText('mock file content')
  })
})
