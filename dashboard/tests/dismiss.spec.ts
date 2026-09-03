import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * One owner of dismissal (bead: chrote-5grx.36).
 *
 * Escape belongs to the topmost open surface and reaches the pty only when
 * nothing is open; a press outside a glance closes it and is consumed; a press
 * outside a work surface is an ordinary press. The pty side is read from the
 * mock socket, because the far end is the only place that can prove what the
 * terminal did or did not send.
 */

/** Everything the client typed at any pty, one entry per ttyd input frame. */
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

/** The one pane the Send drawer resolves before it can offer Send. */
async function mockPanesRoute(page: Page) {
  await page.route('**/api/tmux/sessions/*/panes', async route => {
    const session = decodeURIComponent(new URL(route.request().url()).pathname.split('/')[4])
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        session,
        unixUser: '',
        panes: [{
          sessionId: '$1', pane: '%1', panePid: '4242', serverPid: '9001', windowId: '@1',
          windowName: 'main', currentPath: '/srv/chrote', currentCommand: 'bash', active: true,
        }],
      }),
    })
  })
}

test('Escape closes the topmost surface and never reaches the pane; a press outside closes a glance and not a work surface', async ({ page }) => {
  await mockApiRoutes(page)
  await mockPanesRoute(page)
  const typed = await recordPtyInput(page)
  await page.addInitScript(state => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState())
  await page.goto('/')

  const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
  const first = windows.first()
  const second = windows.nth(1)
  await first.locator('.xterm-screen').click()
  await expect(first).toHaveClass(/focused/)

  // The Keys panel, over a focused tile.
  const panel = page.locator('.keys-panel')
  await page.keyboard.press('Alt+k')
  await expect(panel).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(panel).toBeHidden()

  // A menu, opened on the tile's tag: Escape closes it and nothing else.
  await first.locator('.session-tag').click({ button: 'right' })
  const menu = page.getByRole('menu', { name: /Session actions/ })
  await expect(menu).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(menu).toHaveCount(0)

  // Peek is a glance: a press on the other tile closes it, is consumed, and
  // leaves the focus where it was.
  await first.locator('.xterm-screen').click()
  await page.keyboard.press('Alt+p')
  const peek = page.getByRole('dialog', { name: /^Peek/ })
  await expect(peek).toBeVisible()
  await second.locator('.xterm-screen').click({ position: { x: 20, y: 20 } })
  await expect(peek).toHaveCount(0)
  await expect(second).not.toHaveClass(/focused/)
  await expect(first).toHaveClass(/focused/)

  // Escape inside the peeked terminal closes Peek; the ESC goes to no pty.
  await page.keyboard.press('Alt+p')
  await expect(peek).toBeVisible()
  await peek.locator('.xterm-screen').click()
  await page.keyboard.press('Escape')
  await expect(peek).toHaveCount(0)

  // The Send drawer is work: a press outside leaves it open; Escape closes it.
  await first.locator('.xterm-screen').click()
  await page.keyboard.press('Alt+s')
  const drawer = page.getByRole('dialog', { name: 'Send to session' })
  await expect(drawer).toBeVisible()
  await second.locator('.xterm-screen').click({ position: { x: 20, y: 20 } })
  await expect(drawer).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(drawer).toHaveCount(0)

  // Nothing above was typed at a shell.
  expect(typed.join('')).not.toContain('')

  // With nothing open, Escape is the shell's.
  await first.locator('.xterm-screen').click()
  await page.keyboard.press('Escape')
  await expect.poll(() => typed.join('')).toContain('')
})
