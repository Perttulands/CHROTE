import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Auto-fit regression (beads: chrote-qlx, chrote-g1r, chrote-jkzk.1,
 * chrote-z58o).
 *
 * A terminal must reach tmux already sized to its frame, at the operator's
 * configured font — never at xterm's 80x24 default and never waiting for a
 * manual Refit. The grid is observable directly: it is what the client tells
 * ttyd in its opening handshake, before ttyd spawns a pty.
 */

const TTYD_OUTPUT = 0x30

interface GridEvent extends Handshake {
  kind: 'handshake' | 'resize'
}

interface Handshake { columns: number; rows: number }

/** Record the grid every terminal announces, and answer like a live shell. */
async function recordAnnouncedGrids(page: Page): Promise<GridEvent[]> {
  const grids: GridEvent[] = []
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (text.startsWith('{')) {
        grids.push({ ...JSON.parse(text) as Handshake, kind: 'handshake' })
        ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from('$ ')]))
      } else if (text.startsWith('1')) {
        grids.push({ ...JSON.parse(text.slice(1)) as Handshake, kind: 'resize' })
      }
    })
  })
  return grids
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
  const grids = await recordAnnouncedGrids(page)
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState(fontSize))

  await page.goto('/')
  await expect(page.locator('.terminal-window-body .xterm')).toBeVisible()
  await expect.poll(() => grids.filter(event => event.kind === 'handshake').length, { timeout: 5000 }).toBeGreaterThan(0)
  return grids.find(event => event.kind === 'handshake')!
}

async function expectScreensInsideHosts(page: Page) {
  const overflow = await page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-surface-host:visible').evaluateAll(hosts => (
    hosts.map(host => {
      const hostRect = host.getBoundingClientRect()
      const screenRect = host.querySelector('.xterm-screen')?.getBoundingClientRect()
      return screenRect ? {
        right: screenRect.right - hostRect.right,
        bottom: screenRect.bottom - hostRect.bottom,
      } : null
    })
  ))
  expect(overflow).not.toContain(null)
  overflow.forEach(amount => {
    expect(amount!.right).toBeLessThanOrEqual(0)
    expect(amount!.bottom).toBeLessThanOrEqual(0)
  })
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

  test('refits every existing tile when a third window is added and removed', async ({ page }) => {
    await mockApiRoutes(page)
    const grids = await recordAnnouncedGrids(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, {
      workspaces: {
        terminal1: {
          windowCount: 2,
          windows: [
            { id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 },
            { id: 'terminal1-window-1', boundSessions: ['hq-deacon'], activeSession: 'hq-deacon', colorIndex: 1 },
          ],
        },
        terminal2: { windowCount: 1, windows: [] },
        terminal3: { windowCount: 1, windows: [] },
      },
      sidebarCollapsed: false,
      settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
    })

    await page.goto('/')
    const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
    await expect.poll(() => grids.filter(event => event.kind === 'handshake').length).toBe(2)
    await expectScreensInsideHosts(page)

    grids.length = 0
    await page.keyboard.press('Alt+=')
    await expect(windows).toHaveCount(3)
    await expect.poll(() => grids.filter(event => event.kind === 'resize').length).toBe(2)
    await expectScreensInsideHosts(page)

    grids.length = 0
    await page.keyboard.press('Alt+-')
    await expect(windows).toHaveCount(2)
    await expect.poll(() => grids.filter(event => event.kind === 'resize').length).toBe(2)
    await expectScreensInsideHosts(page)
  })
})
