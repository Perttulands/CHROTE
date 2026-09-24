import { test, expect, type Page } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'
import { openSessionsSidecar } from './helpers'

/**
 * Peek as a centred floating window sized by the session (bead: chrote-5grx.48).
 *
 * The window opens centred over the workspace; Alt+P toggles it and, pressed
 * over another tile, switches it. It holds the tmux window's own grid and
 * fits its font to the room it is given (beads: chrote-8eyu, chrote-wshh):
 * with room, the whole pane is shown and the window wraps it; without, the
 * font stops at a readable floor and the bottom rows stay in view. It opens
 * at one size, and a drag changes the font and never the grid. Real font
 * metrics decide all of that, which is why it is here. The press outside and
 * Escape are the dismissal owner's and are proved in dismiss.spec.ts; what is
 * here is what is Peek's own.
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

/**
 * The case the operator hit: another client sized the window 94x66 with a
 * status line under it. The mock serves that grid for `main`, a pane that
 * fills all 67 rows with the status line on the last, and keeps every size
 * the peek sends, the handshake first, and every key it types.
 */
async function servePeekPane(page: Page) {
  const tall = { name: 'main', windows: 1, attached: true, group: 'main', width: 94, height: 66, statusLines: 1 }
  await mockApiRoutes(page, {
    sessionsResponse: {
      ...mockSessions,
      sessions: mockSessions.sessions.map(session => (session.name === 'main' ? tall : session)),
    },
  })
  const sizes: { cols: number; rows: number }[] = []
  const typed: string[] = []
  const lines = Array.from({ length: 66 }, (_, index) => `pane row ${index + 1}`)
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    if (new URL(ws.url()).searchParams.get('arg') !== 'peek') return
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (text.startsWith('0')) typed.push(text.slice(1))
      const body = text.startsWith('{') ? text : text.startsWith('1') ? text.slice(1) : null
      if (body === null) return
      const size = JSON.parse(body) as { columns: number; rows: number }
      sizes.push({ cols: size.columns, rows: size.rows })
      if (text.startsWith('{')) {
        ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`${lines.join('\r\n')}\r\nSTATUS-LINE-BOTTOM`)]))
      }
    })
  })
  return { sizes, typed }
}

/**
 * The peek's size and font on every animation frame for one second from the
 * first frame it is shown on. Started before the peek is opened, so the first
 * shown frame is in it.
 */
function sampleOpening(page: Page) {
  return page.evaluate(() => new Promise<{ width: number; height: number; font: string }[]>(resolve => {
    const samples: { width: number; height: number; font: string }[] = []
    let shownAt = 0
    const frame = (now: number) => {
      const peek = document.querySelector<HTMLElement>('.peek')
      if (peek && getComputedStyle(peek).opacity !== '0') {
        shownAt ||= now
        const box = peek.getBoundingClientRect()
        const rows = peek.querySelector('.xterm-rows')
        samples.push({ width: box.width, height: box.height, font: rows ? getComputedStyle(rows).fontSize : '' })
        if (now - shownAt >= 1000) {
          resolve(samples)
          return
        }
      }
      requestAnimationFrame(frame)
    }
    requestAnimationFrame(frame)
  }))
}

async function openPeekOnMain(page: Page) {
  await page.goto('/')
  await openSessionsSidecar(page)
  const opening = sampleOpening(page)
  await page.locator('.session-item').filter({ hasText: /^main/ }).first().click()
  return opening
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

  for (const deviceScaleFactor of [1, 1.25, 1.5]) {
    test.describe(`at device scale ${deviceScaleFactor}`, () => {
      test.use({ viewport: { width: 1366, height: 768 }, deviceScaleFactor })

      test('opens at one size, and a pane taller than the room keeps its bottom rows at a readable font', async ({ page }) => {
        const { sizes } = await servePeekPane(page)
        const samples = await openPeekOnMain(page)

        expect(samples.length).toBeGreaterThan(30)
        expect(samples.filter(sample => sample.width !== samples[0].width || sample.height !== samples[0].height || sample.font !== samples[0].font)).toEqual([])
        expect(parseFloat(samples[0].font)).toBeGreaterThanOrEqual(11)

        // The status line and the row above it are inside the body.
        const peek = page.getByRole('dialog', { name: 'Peek main' })
        const body = (await peek.locator('.peek-body').boundingBox())!
        const rows = peek.locator('.xterm-rows > div')
        await expect(rows.last()).toContainText('STATUS-LINE-BOTTOM')
        await expect(rows.nth(-2)).toContainText('pane row 66')
        for (const row of [rows.last(), rows.nth(-2)]) {
          const box = (await row.boundingBox())!
          expect(box.y).toBeGreaterThanOrEqual(body.y - 0.5)
          expect(box.y + box.height).toBeLessThanOrEqual(body.y + body.height + 0.5)
        }
        expect(sizes).toEqual([{ cols: 94, rows: 67 }])
      })
    })
  }

  test.describe('on a 1440p screen', () => {
    test.use({ viewport: { width: 2560, height: 1440 }, deviceScaleFactor: 2 })

    test('wraps the whole pane, is driven from, and a drag changes the font but never the grid', async ({ page }) => {
      const { sizes, typed } = await servePeekPane(page)
      await openPeekOnMain(page)

      const peek = page.getByRole('dialog', { name: 'Peek main' })
      const body = peek.locator('.peek-body')
      const screen = peek.locator('.xterm-screen')
      const statusRow = peek.locator('.xterm-rows > div').last()
      await expect(statusRow).toContainText('STATUS-LINE-BOTTOM')

      // It opened into its own terminal: Escape typed straight away is the
      // session's, and the window stays.
      await expect(peek.locator('.xterm-helper-textarea')).toBeFocused()
      await page.keyboard.press('Escape')
      await expect.poll(() => typed.join('')).toContain('\u001b')
      await expect(peek).toBeVisible()

      // The whole grid is inside the window, and the window holds it with
      // nothing left over: the terminal's padding (4 down, 8 across) and the
      // scrollbar width the fit reserves.
      const bodyBox = (await body.boundingBox())!
      const opened = (await screen.boundingBox())!
      expect(opened.y).toBeGreaterThanOrEqual(bodyBox.y)
      expect(Math.max(bodyBox.height - opened.height - 4, bodyBox.width - opened.width - 8 - 14)).toBeLessThan(1)
      expect(Math.min(bodyBox.height - opened.height - 4, bodyBox.width - opened.width - 8 - 14)).toBeGreaterThanOrEqual(0)

      // Dragged smaller, the font shrinks and the status line stays; the grid holds.
      const frame = (await peek.boundingBox())!
      await page.mouse.move(frame.x + frame.width - 2, frame.y + frame.height - 2)
      await page.mouse.down()
      await page.mouse.move(frame.x + frame.width - 202, frame.y + frame.height - 152, { steps: 10 })
      await page.mouse.up()
      await expect.poll(async () => (await screen.boundingBox())!.height).toBeLessThan(opened.height - 50)
      const dragged = (await body.boundingBox())!
      const row = (await statusRow.boundingBox())!
      expect(row.y + row.height).toBeLessThanOrEqual(dragged.y + dragged.height + 0.5)
      expect(sizes.every(size => size.cols === 94 && size.rows === 67)).toBe(true)
    })
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
