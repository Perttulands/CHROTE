import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * The keyboard model (beads: chrote-5grx.12, chrote-5grx.20).
 *
 * Alt chords run with a terminal focused and never reach the pty, while an
 * unregistered Alt key still does; the leader works the same way and opens the
 * strip, which leads with the Alt column; the keys panel is the registry,
 * searchable; and with keys off the terminal owns every key again. The
 * pass-through cases are read from the mock pty, because the far end is the
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

test('the leader opens the strip over a focused terminal, leading with the Alt column', async ({ page }) => {
  const { typed } = await openWithFocusedTerminal(page)
  const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')

  await page.keyboard.press(LEADER)

  const strip = page.locator('.leader-strip')
  await expect(strip).toBeVisible()
  await expect(strip.locator('.leader-strip-echo')).toHaveText('Ctrl+Shift+Space')
  // The Alt chord is the first column and the bare leader key the second.
  const nextWindow = strip.locator('.leader-strip-chord', { hasText: 'Next window' })
  await expect(nextWindow.locator('.leader-strip-key')).toHaveText('Alt+W')
  await expect(nextWindow.locator('.leader-strip-leader-key')).toHaveText('w')
  await expect(strip.locator('.leader-strip-key').first()).toHaveText('Alt+1')
  // Tile chords are listed because a tile is focused; the scope is real.
  await expect(strip).toContainText('Peek this session')

  await page.keyboard.press('w')

  await expect(strip).toBeHidden()
  await expect(windows.nth(1)).toHaveClass(/focused/)
  await expect(windows.first()).not.toHaveClass(/focused/)
  // Neither the leader nor the chord after it was typed at the shell.
  expect(typed).toEqual([])
})

test('Alt chords run over a focused terminal and unregistered Alt keys reach the pty', async ({ page }) => {
  const { typed } = await openWithFocusedTerminal(page)
  const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')

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

  // Alt+K is the keys panel, and it closes again on Escape.
  await page.keyboard.press('Alt+k')
  await expect(page.locator('.keys-panel')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.locator('.keys-panel')).toBeHidden()

  // Nothing so far was typed at the shell: every one of those was registered.
  expect(typed).toEqual([])

  // Alt+X is not registered, so xterm keeps it and sends the escape sequence.
  await windows.first().locator('.xterm-screen').click()
  await page.keyboard.press('Alt+x')
  await expect.poll(() => typed.join('')).toBe('\u001bx')
})

test('leader then ? opens the keys panel, which searches the registry', async ({ page }) => {
  await openWithFocusedTerminal(page)

  await page.keyboard.press(LEADER)
  await page.keyboard.press('?')

  const panel = page.locator('.keys-panel')
  await expect(panel).toBeVisible()
  await expect(panel.locator('.keys-panel-chord')).toContainText(['Beads tab'])
  const chordCount = await panel.locator('.keys-panel-chord').count()
  expect(chordCount).toBeGreaterThan(5)

  await panel.locator('.keys-panel-search').fill('window')
  const filtered = panel.locator('.keys-panel-chord')
  // The two cycle chords plus the two that change the layout, and nothing else.
  await expect(filtered).toHaveCount(4)
  expect(chordCount).toBeGreaterThan(4)
  await expect(filtered.first()).toContainText('Next window')

  // The search reads the Alt column too, so the chord is findable as the
  // operator writes it.
  await panel.locator('.keys-panel-search').fill('alt+b')
  await expect(panel.locator('.keys-panel-chord')).toHaveCount(1)
  await expect(panel.locator('.keys-panel-chord')).toContainText('Beads tab')

  await panel.locator('.keys-panel-search').fill('no such chord')
  await expect(panel.locator('.keys-panel-empty')).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(panel).toBeHidden()
})

test('with keys off the terminal owns the leader and the key after it', async ({ page }) => {
  const { typed, firstWindow } = await openWithFocusedTerminal(page)

  const keysToggle = page.locator('.keys-menu-container > .tab')
  await expect(keysToggle).toHaveText('Keys on')
  await keysToggle.click()
  await expect(keysToggle).toHaveText('Keys off')

  await firstWindow.locator('.xterm-screen').click()
  await page.keyboard.press(LEADER)
  await page.keyboard.press('a')

  // Nothing was intercepted: xterm answered the leader itself, and the key
  // after it went where every key goes when keys are off.
  await expect(page.locator('.leader-strip')).toHaveCount(0)
  await expect.poll(() => typed.join('')).toContain('a')

  // The toggle brings the model back, and the strip with it.
  await keysToggle.click()
  await expect(keysToggle).toHaveText('Keys on')
  await firstWindow.locator('.xterm-screen').click()
  await page.keyboard.press(LEADER)
  await expect(page.locator('.leader-strip')).toBeVisible()
})
