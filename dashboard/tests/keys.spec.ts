import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * The keyboard model (beads: chrote-5grx.12, chrote-5grx.20, chrote-5grx.22).
 *
 * Alt chords run with a terminal focused and never reach the pty, while an
 * unregistered Alt key still does; the leader is discovery and opens the same
 * keys panel; the panel is the registry, searchable; a chord that fires echoes
 * its caps and is gone; and with keys off the terminal owns every key again.
 * The pass-through cases are read from the mock pty, because the far end is the
 * only place that can prove them.
 */

const LEADER = 'Control+Shift+Space'

/** Everything the client typed at the pty, one entry per ttyd input frame. */
async function recordPtyInput(page: Page): Promise<string[]> {
  const typed: string[] = []
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      // The handshake is bare JSON; '0' is input and '1' is a resize.
      if (text.startsWith('{') || !text.startsWith('0')) return
      typed.push(text.slice(1))
    })
  })
  return typed
}

const emptyWorkspace = (id: string) => ({
  windowCount: 1,
  windows: [{ id: `${id}-window-0`, boundSessions: [], activeSession: null, colorIndex: 0 }],
})

function seededState() {
  return {
    workspaces: {
      terminal1: {
        windowCount: 2,
        windows: [
          { id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
        ],
      },
      terminal2: emptyWorkspace('terminal2'),
      terminal3: emptyWorkspace('terminal3'),
    },
    sidebarCollapsed: false,
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
  }
}

/** Open the dashboard with one bound terminal, and put the cursor in it. */
async function openWithFocusedTerminal(page: Page) {
  await mockApiRoutes(page)
  const typed = await recordPtyInput(page)
  await page.addInitScript(state => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState())
  await page.goto('/')

  const firstWindow = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window').first()
  await firstWindow.locator('.xterm-screen').click()
  await expect(firstWindow).toHaveClass(/focused/)
  return { typed, firstWindow }
}

// One page for the whole keyboard model: the leader is discovery, Alt chords
// run over a focused terminal without reaching the pty, an unregistered Alt key
// still does, and the toggle hands every key back to the terminal.
test('the leader and Alt chords run over a focused terminal, and keys off hands the terminal every key', async ({ page }) => {
  const { typed, firstWindow } = await openWithFocusedTerminal(page)
  const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')

  await page.keyboard.press(LEADER)

  const panel = page.locator('.keys-panel')
  await expect(panel).toBeVisible()
  // Content-sized and centred, with no backdrop between it and the workspace.
  await expect(page.locator('.keys-panel-backdrop')).toHaveCount(0)
  const nextWindow = panel.locator('.keys-panel-chord', { hasText: 'Next window' })
  await expect(nextWindow.locator('.keys-panel-key')).toHaveText('ALT + W')
  // Tile chords are listed because a tile is focused; the scope is real.
  await expect(panel).toContainText("Peek the tile's session")

  // The leader's window is already shut, so the next key is search text.
  await page.keyboard.type('window')
  await expect(panel.locator('.keys-panel-search')).toHaveValue('window')
  await expect(panel.locator('.keys-panel-chord')).toHaveCount(5)

  await page.keyboard.press('Escape')
  await expect(panel).toBeHidden()

  // The leader is the way out as well as the way in.
  await page.keyboard.press(LEADER)
  await expect(panel).toBeVisible()
  await page.keyboard.press(LEADER)
  await expect(panel).toBeHidden()

  // The tab chord, with the cursor in a terminal and no leader first.
  await page.keyboard.press('Alt+2')
  await expect(page.locator('.terminal-workspace-dock[data-workspace="terminal2"]')).toHaveAttribute('data-active', 'true')
  await page.keyboard.press('Alt+1')
  await expect(page.locator('.terminal-workspace-dock[data-workspace="terminal1"]')).toHaveAttribute('data-active', 'true')

  // The window cycle, both directions, on the letter and its Shift form.
  await windows.first().locator('.xterm-screen').click()
  await expect(windows.first()).toHaveClass(/focused/)
  await page.keyboard.press('Alt+w')
  await expect(windows.nth(1)).toHaveClass(/focused/)
  await page.keyboard.press('Alt+Shift+W')
  await expect(windows.first()).toHaveClass(/focused/)

  // Alt+K is the keybindings panel, and pressed again it closes it.
  await page.keyboard.press('Alt+k')
  await expect(panel).toBeVisible()
  await page.keyboard.press('Alt+k')
  await expect(panel).toBeHidden()

  // Nothing so far was typed at the shell: every one of those was registered.
  expect(typed).toEqual([])

  // Alt+X is not registered, so xterm keeps it and sends the escape sequence.
  await windows.first().locator('.xterm-screen').click()
  await page.keyboard.press('Alt+x')
  await expect.poll(() => typed.join('')).toBe('\u001bx')

  const keysToggle = page.locator('.keys-menu-container > .tab')
  await expect(keysToggle).toHaveText('Keys on')
  await keysToggle.click()
  await expect(keysToggle).toHaveText('Keys off')

  await firstWindow.locator('.xterm-screen').click()
  await page.keyboard.press(LEADER)
  await page.keyboard.press('a')

  // Nothing was intercepted: xterm answered the leader itself, and the key
  // after it went where every key goes when keys are off.
  await expect(panel).toHaveCount(0)
  await expect.poll(() => typed.join('')).toContain('a')

  // The toggle brings the model back, and the panel with it.
  await keysToggle.click()
  await expect(keysToggle).toHaveText('Keys on')
  await firstWindow.locator('.xterm-screen').click()
  await page.keyboard.press(LEADER)
  await expect(panel).toBeVisible()
})
