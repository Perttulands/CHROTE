import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { dragAndDrop } from './helpers'

/**
 * Copy-on-select gate (bead: chrote-wgqp.1).
 *
 * Painting a selection in a terminal puts it on the system clipboard. ttyd's
 * client did this and the operator asked for it back when CHROTE took the
 * terminal over. tmux mouse mode is on for CHROTE-created sessions, so the
 * gesture that paints is Shift and left-drag — xterm's own force-selection
 * escape hatch — and the mouse mode is simulated here the way tmux sets it.
 *
 * The insecure-origin half matters as much as the copy: the operator reaches
 * CHROTE over plain HTTP on Tailscale, where navigator.clipboard is absent and
 * only the execCommand fallback in src/utils/clipboard.ts can land the text.
 */

const TTYD_OUTPUT = 0x30
const MOUSE_MODE_ON = '\u001b[?1000h\u001b[?1002h\u001b[?1006h'
const PAINTED_LINE = 'HELLO-CLIPBOARD'

declare global {
  interface Window {
    __realClipboard?: Clipboard
    __copyCommands?: number
  }
}

function seededState() {
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
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
  }
}

/** Answer the handshake with tmux's mouse mode and one line to paint. */
async function serveTerminal(page: Page) {
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (!text.startsWith('{')) return
      ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`${MOUSE_MODE_ON}${PAINTED_LINE}`)]))
    })
  })
}

/**
 * Hide the async Clipboard API from the page the way an insecure origin does,
 * keeping a handle the test itself can still read the clipboard through, and
 * count the copy commands so a per-mousemove copy would be visible.
 */
async function hideAsyncClipboard(page: Page) {
  await page.addInitScript(() => {
    Object.defineProperty(window, '__realClipboard', { value: navigator.clipboard })
    Object.defineProperty(navigator, 'clipboard', { value: undefined, configurable: true })
    window.__copyCommands = 0
    const execCommand = document.execCommand.bind(document)
    document.execCommand = (command: string, ...rest: unknown[]) => {
      if (command === 'copy') window.__copyCommands = (window.__copyCommands ?? 0) + 1
      return execCommand(command, ...(rest as [boolean?, string?]))
    }
  })
}

async function openTerminal(page: Page) {
  await mockApiRoutes(page)
  await serveTerminal(page)
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState())
  await page.goto('/')
  const rows = page.locator('.terminal-window-body .xterm-rows')
  await expect(rows).toContainText(PAINTED_LINE)
  return rows
}

/** Shift and left-drag across the first row, the way the operator paints. */
async function paintFirstRow(page: Page) {
  const row = page.locator('.terminal-window-body .xterm-rows > div').first()
  const box = await row.boundingBox()
  if (!box) throw new Error('no first terminal row')
  const y = box.y + box.height / 2
  await page.keyboard.down('Shift')
  await page.mouse.move(box.x + 1, y)
  await page.mouse.down()
  for (let step = 1; step <= 6; step += 1) {
    await page.mouse.move(box.x + 1 + (box.width - 2) * (step / 6), y)
  }
  await page.mouse.up()
  await page.keyboard.up('Shift')
  return box
}

/** A plain left click in the terminal, the way the operator focuses a tile. */
async function clickFirstRow(page: Page) {
  const row = page.locator('.terminal-window-body .xterm-rows > div').first()
  const box = await row.boundingBox()
  if (!box) throw new Error('no first terminal row')
  await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
}

function readClipboard(page: Page) {
  return page.evaluate(() => window.__realClipboard?.readText() ?? navigator.clipboard.readText())
}

test.describe('Terminal copy on select', () => {
  test.beforeEach(async ({ context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
  })

  test('a painted selection lands on the clipboard through the async Clipboard API', async ({ page }) => {
    await openTerminal(page)
    await page.evaluate(() => navigator.clipboard.writeText('untouched'))

    await paintFirstRow(page)

    await expect.poll(() => readClipboard(page)).toContain(PAINTED_LINE)
  })

  // The plain-HTTP path is the one the operator actually reaches CHROTE on, so
  // every rule about when a copy fires is proven here, on one page: painting
  // copies once, and a click — over an old selection or over none — never does.
  test('copies once per painted drag on a plain-HTTP origin, and never on a click', async ({ page }) => {
    await hideAsyncClipboard(page)
    await openTerminal(page)
    await page.evaluate(() => window.__realClipboard?.writeText('untouched'))

    await clickFirstRow(page)
    expect(await page.evaluate(() => window.__copyCommands)).toBe(0)

    await paintFirstRow(page)

    await expect.poll(() => readClipboard(page)).toContain(PAINTED_LINE)
    // Once for the whole drag, not once per mousemove.
    expect(await page.evaluate(() => window.__copyCommands)).toBe(1)

    // The click lands while the painted selection is still on screen; putting
    // it back on the clipboard would overwrite whatever the operator copied.
    await clickFirstRow(page)
    expect(await page.evaluate(() => window.__copyCommands)).toBe(1)
  })

  // Moved from the live suite, which needed a real backend for none of it. The
  // terminal shares the dashboard's document, so only this assertion stops the
  // dashboard from swallowing the operator's native right-click menu inside it.
  test('keeps terminal input native while exposing visible assignment controls', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await mockApiRoutes(page)
    await page.goto('/')
    await expect(page.locator('.dashboard')).toBeVisible()
    await expect(page.locator('.terminal-grid[data-workspace="terminal1"]')).toBeVisible()

    // Tab out of the tab bar: the active tab stays active and gives up focus,
    // rather than the tab strip capturing the keyboard.
    const terminalOneTab = page.locator('.tab-bar .tab.active').first()
    await expect(terminalOneTab).toHaveClass(/active/)
    await terminalOneTab.focus()
    await page.keyboard.press('Tab')
    await expect(terminalOneTab).toHaveClass(/active/)
    await expect(terminalOneTab).not.toBeFocused()

    const workspace = page.locator('.terminal-workspace-dock[data-active="true"]')
    const sessionsSidecar = workspace.getByRole('button', { name: 'Sessions sidecar', exact: true })
    await expect(sessionsSidecar).toHaveAttribute('aria-expanded', 'false')
    await sessionsSidecar.click()
    await expect(sessionsSidecar).toHaveAttribute('aria-expanded', 'true')

    const sessionRow = workspace.locator('.session-item').first()
    await expect(sessionRow).toBeVisible()
    const sessionName = (await sessionRow.locator('.session-name').textContent())?.trim()
    expect(sessionName).toBeTruthy()
    await expect(sessionRow.getByRole('button', { name: `Session actions for ${sessionName}` })).toBeVisible()

    const firstWindow = workspace.locator('.terminal-window:visible').first()
    await dragAndDrop(page, '.session-panel .session-item', '.terminal-window')
    await expect(firstWindow.locator('.tag-name')).toHaveText(sessionName!)

    await sessionsSidecar.click()
    await expect(sessionsSidecar).toHaveAttribute('aria-expanded', 'false')

    const terminal = firstWindow.locator('.terminal-window-body .terminal-surface')
    await expect(terminal.locator('.xterm')).toBeVisible()

    await terminal.evaluate(element => {
      const marker = window as Window & { __chroteContextMenuPrevented?: boolean }
      marker.__chroteContextMenuPrevented = true
      element.addEventListener('contextmenu', event => {
        marker.__chroteContextMenuPrevented = event.defaultPrevented
      }, { once: true })
    })
    await terminal.click({ button: 'right', position: { x: 20, y: 20 } })
    await expect.poll(() => page.evaluate(() => (
      window as Window & { __chroteContextMenuPrevented?: boolean }
    ).__chroteContextMenuPrevented)).toBe(false)
  })
})
