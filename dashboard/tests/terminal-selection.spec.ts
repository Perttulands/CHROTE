import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

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

  test('copies on a plain-HTTP origin, where only execCommand is left', async ({ page }) => {
    await hideAsyncClipboard(page)
    await openTerminal(page)
    await page.evaluate(() => window.__realClipboard?.writeText('untouched'))

    await paintFirstRow(page)

    await expect.poll(() => readClipboard(page)).toContain(PAINTED_LINE)
  })

  test('copies once when the drag settles, not once per mousemove', async ({ page }) => {
    await hideAsyncClipboard(page)
    await openTerminal(page)

    await paintFirstRow(page)

    expect(await page.evaluate(() => window.__copyCommands)).toBe(1)
  })

  test('a click that paints nothing copies nothing', async ({ page }) => {
    await hideAsyncClipboard(page)
    await openTerminal(page)
    await clickFirstRow(page)

    expect(await page.evaluate(() => window.__copyCommands)).toBe(0)
  })

  test('clicking a terminal that still holds an old selection does not put it back', async ({ page }) => {
    await hideAsyncClipboard(page)
    await openTerminal(page)
    await paintFirstRow(page)
    expect(await page.evaluate(() => window.__copyCommands)).toBe(1)

    await clickFirstRow(page)

    expect(await page.evaluate(() => window.__copyCommands)).toBe(1)
  })
})
