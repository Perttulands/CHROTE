import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { openSessionsSidecar } from './helpers'

/**
 * Peek as a centred floating window sized by the session (bead: chrote-5grx.48).
 *
 * The window opens centred over the workspace at the width of the session's
 * own grid, capped at 70% of the workspace; Alt+P toggles it and, pressed
 * over another tile, switches it. The press outside and Escape from inside
 * the peeked terminal are the dismissal owner's and are proved in
 * dismiss.spec.ts; what is here is what is Peek's own.
 */

const TTYD_OUTPUT = 0x30
const MOUSE_MODE_ON = '[?1000h[?1002h[?1006h'
const PEEK_LINE = 'PEEK-SELECT-ME'

/**
 * A ttyd stand-in that answers every handshake with tmux's mouse mode and one
 * line to paint, and keeps the latest columns each viewer asked for, keyed by
 * its mode and session: the handshake first, then every resize after it.
 */
async function serveTerminals(page: Page) {
  const columns: Record<string, number> = {}
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    const [mode, name] = new URL(ws.url()).searchParams.getAll('arg')
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (text.startsWith('{')) {
        columns[`${mode}:${name}`] = (JSON.parse(text) as { columns: number }).columns
        ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`${MOUSE_MODE_ON}${PEEK_LINE}`)]))
      } else if (text.startsWith('1')) {
        columns[`${mode}:${name}`] = (JSON.parse(text.slice(1)) as { columns: number }).columns
      }
    })
  })
  return columns
}

function seededState() {
  return {
    workspaces: {
      terminal1: {
        windowCount: 2,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: ['gt-gastown-jack'], activeSession: 'gt-gastown-jack', colorIndex: 1 },
        ],
      },
      terminal2: { windowCount: 1, windows: [] },
      terminal3: { windowCount: 1, windows: [] },
    },
    sidebarCollapsed: false,
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
  }
}

test.describe('Peek', () => {
  test('opens centred at the session\'s width from Alt+P, toggles on it, and switches from another tile', async ({ page }) => {
    await mockApiRoutes(page)
    const columns = await serveTerminals(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')

    const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
    await windows.first().locator('.xterm-screen').click()
    await expect(windows.first()).toHaveClass(/focused/)
    await expect.poll(() => columns['tile:main']).toBeGreaterThan(0)

    await page.keyboard.press('Alt+p')
    const peek = page.locator('.peek')
    await expect(peek).toBeVisible()
    await expect(peek.locator('.peek-name')).toHaveText('main')

    const peekBox = (await peek.boundingBox())!
    const workspaceBox = (await page.locator('.dashboard-content').boundingBox())!
    // Centred over the workspace, and never more than 70% of it wide.
    expect(Math.abs((peekBox.x + peekBox.width / 2) - (workspaceBox.x + workspaceBox.width / 2))).toBeLessThanOrEqual(1)
    expect(Math.abs((peekBox.y + peekBox.height / 2) - (workspaceBox.y + workspaceBox.height / 2))).toBeLessThanOrEqual(1)
    expect(peekBox.width).toBeLessThanOrEqual(workspaceBox.width * 0.7 + 1)
    expect(peekBox.height).toBeLessThanOrEqual(workspaceBox.height * 0.8 + 1)
    // At the session's width: the peek's grid asks for the tile's columns.
    await expect.poll(() => columns['peek:main']).toBe(columns['tile:main'])

    // The same chord over the same tile closes it.
    await page.keyboard.press('Alt+p')
    await expect(peek).toHaveCount(0)

    // Over another tile it opens on that tile's session, and switches to it
    // while open.
    await page.keyboard.press('Alt+p')
    await expect(peek.locator('.peek-name')).toHaveText('main')
    await page.keyboard.press('Alt+w')
    await expect(windows.nth(1)).toHaveClass(/focused/)
    await page.keyboard.press('Alt+p')
    await expect(peek.locator('.peek-name')).toHaveText('gt-gastown-jack')
    await page.keyboard.press('Alt+p')
    await expect(peek).toHaveCount(0)
  })

  test('opens from a session row, keeps a selection released outside, and closes from its Close word', async ({ page }) => {
    await mockApiRoutes(page)
    await serveTerminals(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page)
    await page.waitForSelector('.session-item')
    await page.click('.session-item:has-text("jack")')

    const peek = page.locator('.peek')
    await expect(peek).toBeVisible()
    const rows = peek.locator('.xterm-rows')
    await expect(rows).toContainText(PEEK_LINE)

    // A selection painted in the peeked terminal and released outside the
    // window keeps both the selection and the window: the press began inside.
    const row = peek.locator('.xterm-rows > div').first()
    const box = (await row.boundingBox())!
    const y = box.y + box.height / 2
    await page.keyboard.down('Shift')
    await page.mouse.move(box.x + box.width / 2, y)
    await page.mouse.down()
    await page.mouse.move(box.x + 1, y, { steps: 5 })
    await page.mouse.move(4, y, { steps: 5 })
    await page.mouse.up()
    await page.keyboard.up('Shift')

    await expect(peek).toBeVisible()
    await expect(peek.locator('.xterm-selection > div')).not.toHaveCount(0)

    await peek.getByRole('button', { name: 'Close' }).click()
    await expect(peek).toHaveCount(0)
  })
})
