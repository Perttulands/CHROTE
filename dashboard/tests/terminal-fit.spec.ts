import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Auto-fit regression (beads: chrote-qlx, chrote-g1r, chrote-jkzk.1).
 *
 * A terminal must reach tmux already sized to its frame, at the operator's
 * configured font — never at xterm's 80x24 default and never waiting for a
 * manual Refit. The grid is observable directly: it is what the client tells
 * ttyd in its opening handshake, before ttyd spawns a pty.
 */

const TTYD_OUTPUT = 0x30

interface Handshake { columns: number; rows: number }

/** Record the grid every terminal announces, and answer like a live shell. */
async function recordAnnouncedGrids(page: Page): Promise<Handshake[]> {
  const handshakes: Handshake[] = []
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (!text.startsWith('{')) return
      handshakes.push(JSON.parse(text) as Handshake)
      ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from('$ ')]))
    })
  })
  return handshakes
}

function seededState(fontSize: number) {
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
    settings: { theme: 'dark', fontSize, autoRefreshInterval: 1000 },
  }
}

async function openTerminalWithFontSize(page: Page, fontSize: number): Promise<Handshake> {
  await mockApiRoutes(page)
  const handshakes = await recordAnnouncedGrids(page)
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState(fontSize))

  await page.goto('/')
  await expect(page.locator('.terminal-window-body .xterm')).toBeVisible()
  await expect.poll(() => handshakes.length, { timeout: 5000 }).toBeGreaterThan(0)
  return handshakes[0]
}

test.describe('Terminal auto-fit', () => {
  // Both halves on one page: a terminal must reach tmux fitted to its frame,
  // and it must apply the configured font before it measures that fit.
  test('announces a grid fitted to the frame, at the configured font, with no manual Refit', async ({ page }) => {
    const small = await openTerminalWithFontSize(page, 14)

    // xterm's untouched default is 80x24; a fitted full-width tile is much wider.
    expect(small.columns).toBeGreaterThan(80)
    expect(small.rows).toBeGreaterThan(24)

    const large = await openTerminalWithFontSize(page, 28)

    // Fitting with stale cell metrics was the clipped-input-row bug: a bigger
    // font must produce a smaller grid in the same frame.
    expect(large.columns).toBeLessThan(small.columns)
    expect(large.rows).toBeLessThan(small.rows)
  })
})
