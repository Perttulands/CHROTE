import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Reparent retention gate (bead: chrote-g1r).
 *
 * Shrinking the window count unmounts a TerminalWindow, which releases its
 * claimed iframe back to the hidden pool; growing re-claims it. Both are
 * physical DOM reparents. With appendChild those moves RELOAD the iframe
 * document in real Chromium (killing the ttyd WebSocket and wiping state);
 * with moveBefore (Chrome 133+) the document survives. This spec plants a
 * marker inside the iframe document and counts /terminal document loads
 * across a 2 -> 1 -> 2 window-count round trip: exactly one load, marker
 * intact. Fails against appendChild-based claiming.
 * Evidence: /srv/data/chrote/evidence/fit-probe-20260803/ (probe2/probe3).
 */

const SEEDED_STATE = {
  workspaces: {
    terminal1: {
      windowCount: 2,
      windows: [
        { id: 'terminal1-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
        { id: 'terminal1-window-1', boundSessions: ['main'], activeSession: 'main', colorIndex: 1 },
      ],
    },
    terminal2: { windowCount: 1, windows: [] },
    terminal3: { windowCount: 1, windows: [] },
  },
  sidebarCollapsed: false,
  settings: {
    theme: 'dark',
    fontSize: 14,
    autoRefreshInterval: 1000,
  },
}

test.describe('Terminal reparent retention', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.addInitScript((state) => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, SEEDED_STATE)
  })

  test('iframe document survives window-count 2 -> 1 -> 2 park/reclaim reparents without reloading', async ({ page }) => {
    const terminalDocumentLoads: string[] = []
    page.on('request', request => {
      if (request.resourceType() === 'document' && request.url().includes('/terminal/')) {
        terminalDocumentLoads.push(request.url())
      }
    })

    await page.goto('/')

    const frameElement = page.locator('.terminal-grid[data-workspace="terminal1"] iframe')
    await expect(frameElement).toBeVisible()
    await expect
      .poll(async () => frameElement.evaluate((el: HTMLIFrameElement) => el.src))
      .toContain('/terminal/?arg=main')

    // Marker inside the iframe document: any reload or browsing-context
    // recreation wipes it. Plant only once the /terminal document has
    // committed and finished loading — stamping the transient about:blank
    // window would vanish on navigation commit and fake a reload.
    await expect
      .poll(async () => frameElement.evaluate((el: HTMLIFrameElement) => {
        const win = el.contentWindow as (Window & { __reparentMarker?: string }) | null
        if (!win || !win.location.href.includes('/terminal/')) return 'not-committed'
        if (win.document.readyState !== 'complete') return 'still-loading'
        win.__reparentMarker = 'alive'
        return 'alive'
      }))
      .toBe('alive')

    const activeDock = page.locator('[data-active="true"]')

    // Shrink: terminal1-window-1 unmounts, its iframe parks in the pool.
    await activeDock.getByTitle('1 window').click()
    await expect(frameElement).toBeHidden()

    // Grow: the window remounts and re-claims the parked iframe.
    await activeDock.getByTitle('2 windows').click()
    await expect(frameElement).toBeVisible()

    const marker = await frameElement.evaluate((el: HTMLIFrameElement) => {
      const win = el.contentWindow as (Window & { __reparentMarker?: string }) | null
      return win?.__reparentMarker ?? 'gone'
    })
    expect(marker).toBe('alive')

    // Exactly one document load for the whole round trip — StrictMode claim
    // churn and both reparents included. appendChild-based moves reload on
    // every reparent and fail this.
    expect(terminalDocumentLoads).toHaveLength(1)
  })
})
