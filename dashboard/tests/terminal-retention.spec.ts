import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Retention gate (beads: chrote-g1r, chrote-9bf, chrote-jkzk.1).
 *
 * A terminal taken off screen keeps its connection and its rendered frame.
 * Both routes off screen are covered: shrinking the window count inside a
 * workspace, and shrinking the settings-controlled terminal tab count. Either
 * one reconnecting would drop the operator's session output and hand tmux a
 * second client.
 */

const TTYD_OUTPUT = 0x30

/** Count terminal connections and stamp each one into the rendered output. */
async function trackTerminalConnections(page: Page) {
  const connections = { count: 0 }
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    connections.count += 1
    const generation = connections.count
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (!text.startsWith('{')) return
      ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`connection-${generation}`)]))
    })
  })
  return connections
}

function seededState(workspaces: Record<string, unknown>) {
  return {
    workspaces,
    sidebarCollapsed: false,
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
  }
}

const emptyWorkspace = (id: string) => ({
  windowCount: 1,
  windows: [{ id: `${id}-window-0`, boundSessions: [], activeSession: null, colorIndex: 0 }],
})

async function open(page: Page, workspaces: Record<string, unknown>) {
  await mockApiRoutes(page)
  const connections = await trackTerminalConnections(page)
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState(workspaces))
  await page.goto('/')
  return connections
}

test.describe('Terminal retention', () => {
  // The counts left the strip, so the layout moves on its chords. A live
  // terminal keeps its one connection across every step, and the chord that
  // shrinks the layout stops at the window holding it.
  test('survives a window-count grow and shrink inside a workspace', async ({ page }) => {
    const connections = await open(page, {
      terminal1: {
        windowCount: 2,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
        ],
      },
      terminal2: emptyWorkspace('terminal2'),
      terminal3: emptyWorkspace('terminal3'),
    })

    const terminal = page.locator('.terminal-grid[data-workspace="terminal1"] .xterm-rows')
    const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
    const controls = page.locator('.terminal-area:visible .terminal-area-controls')
    await expect(terminal).toContainText('connection-1')

    // The strip carries no buttons: it names the chords and states the count
    // they reached, which is the only place the number is readable.
    await expect(controls).toContainText('Alt+= add window · Alt+- remove empty')
    await expect(controls.locator('.layout-count')).toHaveText('2')
    await expect(controls.locator('.layout-btn')).toHaveCount(0)

    await page.keyboard.press('Alt+=')
    await expect(windows).toHaveCount(3)
    await page.keyboard.press('Alt+=')
    await expect(windows).toHaveCount(4)
    await expect(page.locator('.terminal-grid:visible')).toHaveClass(/grid-4/)
    await expect(controls.locator('.layout-count')).toHaveText('4')

    // The tiles share the frame rather than one of them keeping most of it.
    const firstBox = await windows.nth(0).boundingBox()
    const thirdBox = await windows.nth(2).boundingBox()
    expect(firstBox).toBeTruthy()
    expect(thirdBox).toBeTruthy()
    expect(Math.abs(firstBox!.height - thirdBox!.height)).toBeLessThan(10)

    for (let press = 0; press < 3; press += 1) await page.keyboard.press('Alt+-')
    await expect(windows).toHaveCount(1)
    await expect(page.locator('.terminal-grid:visible')).toHaveClass(/grid-1/)
    await expect(controls.locator('.layout-count')).toHaveText('1')

    // The last window holds somebody's live terminal, so the chord refuses.
    await page.keyboard.press('Alt+-')
    await expect(windows).toHaveCount(1)

    await expect(terminal).toContainText('connection-1')
    expect(connections.count).toBe(1)
  })

  test('survives a terminal tab-count shrink and grow, with session refreshes landing meanwhile', async ({ page }) => {
    const connections = await open(page, {
      terminal1: emptyWorkspace('terminal1'),
      terminal2: emptyWorkspace('terminal2'),
      terminal3: {
        windowCount: 1,
        windows: [{ id: 'terminal3-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 }],
      },
    })

    await page.getByRole('button', { name: 'Terminal 3' }).click()
    const terminal = page.locator('.terminal-grid[data-workspace="terminal3"] .xterm-rows')
    await expect(terminal).toContainText('connection-1')

    await page.getByRole('button', { name: 'Settings' }).click()
    await page.getByRole('combobox', { name: 'Terminal tabs' }).selectOption('2')
    await expect(page.getByRole('button', { name: 'Terminal 3' })).toHaveCount(0)

    // Adversarial window: session refreshes must land while the workspace is
    // unreachable. A visibility-derived binding list would let the pool
    // dispose the terminal here.
    await page.waitForRequest(request => request.url().includes('/api/tmux/sessions'))
    await page.waitForRequest(request => request.url().includes('/api/tmux/sessions'))

    await page.getByRole('button', { name: 'Terminal', exact: true }).click()
    await page.getByRole('button', { name: 'Settings' }).click()
    await page.getByRole('combobox', { name: 'Terminal tabs' }).selectOption('3')
    await page.getByRole('button', { name: 'Terminal 3' }).click()

    await expect(terminal).toContainText('connection-1')
    expect(connections.count).toBe(1)
  })
})
