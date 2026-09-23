import { test, expect, type Page } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'
import { openSessionsSidecar } from './helpers'

/**
 * Peek as a centred floating window sized by the session (bead: chrote-5grx.48).
 *
 * The window opens centred over the workspace; Alt+P toggles it and, pressed
 * over another tile, switches it. It shows the tmux window whole, at the
 * window's own grid, and fits its font to its box (bead: chrote-8eyu): the
 * bottom row of a pane taller than the box is on screen, and a drag changes
 * the font and never the grid. Real font metrics decide that, which is why it
 * is here. The press outside and Escape are the dismissal owner's and are
 * proved in dismiss.spec.ts; what is here is what is Peek's own.
 *
 * The size a corner is dragged to is the size every later peek opens at, on
 * whatever session (bead: chrote-mc8d). A real pointer drag through pointer
 * capture needs a real browser, which is why that one is here.
 */

const TTYD_OUTPUT = 0x30
const MOUSE_MODE_ON = '\u001b[?1000h\u001b[?1002h\u001b[?1006h'
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
  test('opens centred from Alt+P, toggles on it, and switches from another tile', async ({ page }) => {
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
    // Centred over the workspace.
    expect(Math.abs((peekBox.x + peekBox.width / 2) - (workspaceBox.x + workspaceBox.width / 2))).toBeLessThanOrEqual(1)
    expect(Math.abs((peekBox.y + peekBox.height / 2) - (workspaceBox.y + workspaceBox.height / 2))).toBeLessThanOrEqual(1)

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

  test('shows every row of a pane taller than its box, and a drag changes the font but never the grid', async ({ page }) => {
    // The case the operator hit: another client sized the window 94x66 with a
    // status line under it, on a 1080p-class screen.
    await page.setViewportSize({ width: 1920, height: 1080 })
    const tall = { name: 'main', windows: 1, attached: true, group: 'main', width: 94, height: 66, statusLines: 1 }
    await mockApiRoutes(page, {
      sessionsResponse: {
        ...mockSessions,
        sessions: mockSessions.sessions.map(session => (session.name === 'main' ? tall : session)),
      },
    })
    // Every size the peek sends, the handshake first; and a pane that fills
    // all 67 rows, with the status line on the last one.
    const sizes: { cols: number; rows: number }[] = []
    const lines = Array.from({ length: 66 }, (_, index) => `pane row ${index + 1}`)
    await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
      if (new URL(ws.url()).searchParams.get('arg') !== 'peek') return
      ws.onMessage(message => {
        const text = typeof message === 'string' ? message : message.toString('utf8')
        const body = text.startsWith('{') ? text : text.startsWith('1') ? text.slice(1) : null
        if (body === null) return
        const size = JSON.parse(body) as { columns: number; rows: number }
        sizes.push({ cols: size.columns, rows: size.rows })
        if (text.startsWith('{')) {
          ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`${lines.join('\r\n')}\r\nSTATUS-LINE-BOTTOM`)]))
        }
      })
    })
    await page.goto('/')
    await openSessionsSidecar(page)
    await page.locator('.session-item').filter({ hasText: /^main/ }).first().click()

    const peek = page.getByRole('dialog', { name: 'Peek main' })
    const body = peek.locator('.peek-body')
    const statusRow = peek.locator('.xterm-rows > div').last()
    await expect(statusRow).toContainText('STATUS-LINE-BOTTOM')
    expect(sizes[0]).toEqual({ cols: 94, rows: 67 })

    // The whole grid is inside the window: the last row and the right edge.
    const inside = async () => {
      const box = (await body.boundingBox())!
      const screen = (await peek.locator('.xterm-screen').boundingBox())!
      const row = (await statusRow.boundingBox())!
      expect(screen.x + screen.width).toBeLessThanOrEqual(box.x + box.width + 0.5)
      expect(screen.y + screen.height).toBeLessThanOrEqual(box.y + box.height + 0.5)
      expect(row.y + row.height).toBeLessThanOrEqual(box.y + box.height + 0.5)
      return screen
    }
    const opened = await inside()

    // Dragged smaller, the font shrinks to keep every row; the grid holds.
    const frame = (await peek.boundingBox())!
    await page.mouse.move(frame.x + frame.width - 2, frame.y + frame.height - 2)
    await page.mouse.down()
    await page.mouse.move(frame.x + frame.width - 202, frame.y + frame.height - 152, { steps: 10 })
    await page.mouse.up()
    await expect.poll(async () => (await peek.locator('.xterm-screen').boundingBox())!.height).toBeLessThan(opened.height - 50)
    await inside()
    await expect(statusRow).toContainText('STATUS-LINE-BOTTOM')
    expect(sizes.every(size => size.cols === 94 && size.rows === 67)).toBe(true)
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

  test('keeps the size a corner was dragged to for the next peek, on another session', async ({ page }) => {
    await mockApiRoutes(page)
    await serveTerminals(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')

    const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
    await windows.first().locator('.xterm-screen').click()
    await expect(windows.first()).toHaveClass(/focused/)

    const peek = page.locator('.peek')
    await page.keyboard.press('Alt+p')
    await expect(peek).toBeVisible()
    await expect(peek.locator('.peek-name')).toHaveText('main')
    const opened = (await peek.boundingBox())!

    // Drag the bottom-right corner in. The window is centred, so it shrinks
    // around its middle by twice the pointer's travel.
    await page.mouse.move(opened.x + opened.width - 2, opened.y + opened.height - 2)
    await page.mouse.down()
    await page.mouse.move(opened.x + opened.width - 152, opened.y + opened.height - 102, { steps: 10 })
    await page.mouse.up()

    const dragged = (await peek.boundingBox())!
    expect(dragged.width).toBeLessThan(opened.width - 200)
    expect(dragged.height).toBeLessThan(opened.height - 100)

    // Closed, and reopened on the other session: the operator sized the
    // window, not the session inside it.
    await peek.getByRole('button', { name: 'Close' }).click()
    await expect(peek).toHaveCount(0)
    await page.keyboard.press('Alt+w')
    await expect(windows.nth(1)).toHaveClass(/focused/)
    await page.keyboard.press('Alt+p')
    await expect(peek.locator('.peek-name')).toHaveText('gt-gastown-jack')

    const reopened = (await peek.boundingBox())!
    expect(Math.round(reopened.width)).toBe(Math.round(dragged.width))
    expect(Math.round(reopened.height)).toBe(Math.round(dragged.height))

    // The word in the header gives the session the say back.
    await peek.getByRole('button', { name: 'Reset size' }).click()
    await expect.poll(async () => (await peek.boundingBox())!.width).toBeGreaterThan(dragged.width)
  })
})
