import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { dragAndDrop } from './helpers'

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
  const connections = {
    count: 0,
    sessions: new Map<string, {
      grids: { columns: number; rows: number }[]
      send: (text: string) => void
    }>(),
  }
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    connections.count += 1
    const generation = connections.count
    const sessionName = new URL(ws.url()).searchParams.getAll('arg')[1]!
    const terminal = {
      grids: [] as { columns: number; rows: number }[],
      send: (text: string) => ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(text)])),
    }
    connections.sessions.set(sessionName, terminal)
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (text.startsWith('{')) {
        terminal.grids.push(JSON.parse(text))
        terminal.send(`connection-${generation}`)
      } else if (text.startsWith('1')) {
        terminal.grids.push(JSON.parse(text.slice(1)))
      }
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
  test('opens saved bindings at first visible geometry and keeps their connection and output across tags, workspaces and moves', async ({ page }) => {
    const names = [['main', 'hq-deacon'], ['hq-mayor', 'gt-gastown-jack'], ['gt-gastown-joe', 'gt-gastown-max']]
    const connections = await open(page, Object.fromEntries(names.map((boundSessions, index) => {
      const id = `terminal${index + 1}`
      return [id, {
        windowCount: 1,
        windows: [{ id: `${id}-window-0`, boundSessions, activeSession: boundSessions[0], colorIndex: 0 }],
      }]
    })))
    const firstWorkspace = '.terminal-grid[data-workspace="terminal1"]'
    const visibleRows = () => page.locator('.terminal-surface-host:visible .xterm-rows')
    await expect(visibleRows()).toContainText('connection-1')
    // A session poll lands after startup effects, so all restored bindings have
    // had a chance to attach. Only the displayed binding may have done so.
    await page.waitForRequest(request => request.url().includes('/api/tmux/sessions'))
    expect(connections.count).toBe(1)
    const main = connections.sessions.get('main')!
    expect(main.grids[0].columns).toBeGreaterThan(80)
    expect(main.grids[0].rows).toBeGreaterThan(24)
    await visibleRows().evaluate(element => { element.setAttribute('data-retained-frame', 'main') })

    await page.locator(`${firstWorkspace} .session-tag`).filter({ hasText: 'hq-deacon' }).click()
    await expect(visibleRows()).toContainText('connection-2')
    expect(connections.count).toBe(2)
    const other = connections.sessions.get('hq-deacon')!
    expect(other.grids[0]).toEqual(main.grids[0])
    main.send('\r\noutput while its tag was hidden')
    const mainGridsWhileHidden = main.grids.length

    await page.getByRole('button', { name: 'Terminal 2' }).click()
    await expect(visibleRows()).toContainText('connection-3')
    expect(connections.count).toBe(3)
    expect(connections.sessions.get('hq-mayor')!.grids[0]).toEqual(main.grids[0])
    main.send('\r\noutput while its workspace was hidden')
    expect(main.grids).toHaveLength(mainGridsWhileHidden)

    await page.getByRole('button', { name: 'Terminal', exact: true }).click()
    await expect(visibleRows()).toContainText('connection-2')
    await page.locator(`${firstWorkspace} .session-tag`).filter({ hasText: 'main' }).click()
    await expect(visibleRows()).toContainText('output while its tag was hidden')
    await expect(visibleRows()).toContainText('output while its workspace was hidden')
    await expect(visibleRows()).toHaveAttribute('data-retained-frame', 'main')

    await page.keyboard.press('Alt+=')
    await expect(page.locator(`${firstWorkspace} .terminal-window`)).toHaveCount(2)
    await dragAndDrop(page, `${firstWorkspace} .session-tag:has-text("main")`, `${firstWorkspace} .terminal-window:nth-child(2) .terminal-window-body`)
    const movedRows = page.locator(`${firstWorkspace} .terminal-window`).nth(1).locator('.xterm-rows')
    await expect(movedRows).toHaveAttribute('data-retained-frame', 'main')
    await expect(movedRows).toContainText('output while its workspace was hidden')
    main.send('\r\nstill on the original connection after moving')
    await expect(movedRows).toContainText('still on the original connection after moving')
    expect(connections.count).toBe(3)
  })

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
