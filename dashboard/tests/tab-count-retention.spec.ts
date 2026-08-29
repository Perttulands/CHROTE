import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Browser retention gate for the settings-controlled terminal tab count.
 *
 * A pooled terminal iframe claimed in terminal3 must survive shrinking the
 * tab count to 2 and growing back to 3: same document (no reload, no src
 * reset, no reconnect), while session refreshes keep landing and the user
 * moves between tabs. A reload or recreation would wipe the marker planted
 * inside the iframe document and issue a second /terminal document request.
 *
 * Bead: chrote-9bf
 */

const SEEDED_STATE = {
  workspaces: {
    terminal1: {
      windowCount: 2,
      windows: [
        { id: 'terminal1-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
        { id: 'terminal1-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
      ],
    },
    terminal2: {
      windowCount: 2,
      windows: [
        { id: 'terminal2-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
        { id: 'terminal2-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
      ],
    },
    terminal3: {
      windowCount: 1,
      windows: [
        { id: 'terminal3-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 },
      ],
    },
  },
  sidebarCollapsed: false,
  settings: {
    theme: 'dark',
    fontSize: 14,
    autoRefreshInterval: 1000,
  },
}

test.describe('Tab count retention', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.addInitScript((state) => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, SEEDED_STATE)
  })

  test('terminal3 iframe survives shrink to 2 and grow back to 3 without reloading', async ({ page }) => {
    const terminalDocumentLoads: string[] = []
    page.on('request', request => {
      if (request.resourceType() === 'document' && request.url().includes('/terminal/')) {
        terminalDocumentLoads.push(request.url())
      }
    })

    await page.goto('/')

    // Claim the iframe by visiting terminal3; first claim sets src.
    await page.getByRole('button', { name: 'Terminal 3' }).click()
    const frameElement = page.locator('.terminal-grid[data-workspace="terminal3"] iframe')
    await expect(frameElement).toBeVisible()
    await expect
      .poll(async () => frameElement.evaluate((el: HTMLIFrameElement) => el.src))
      .toContain('/terminal/?arg=main')

    // Plant markers: one inside the iframe document (dies on any reload or
    // node recreation) and one on the element object (dies on recreation).
    await frameElement.evaluate((el: HTMLIFrameElement) => {
      ;(el as HTMLIFrameElement & { __retentionTag?: string }).__retentionTag = 'original-node'
      const win = el.contentWindow as (Window & { __retentionMarker?: string }) | null
      if (!win) throw new Error('iframe has no contentWindow')
      win.__retentionMarker = 'alive'
    })
    const initialSrc = await frameElement.evaluate((el: HTMLIFrameElement) => el.src)
    // Initial claim/mount churn (StrictMode double-effects re-parent the
    // iframe in the dev harness) can load the document more than once; the
    // gate is that the shrink/grow cycle below adds ZERO loads beyond this
    // baseline — the markers planted above prove the document never reloaded.
    const baselineLoads = terminalDocumentLoads.filter(url => url.includes('arg=main')).length

    // Shrink to 2 tabs through the real settings surface.
    await page.getByRole('button', { name: 'Settings' }).click()
    await page.getByRole('combobox', { name: 'Terminal tabs' }).selectOption('2')
    await expect(page.getByRole('button', { name: 'Terminal 3' })).toHaveCount(0)

    // Adversarial window: at least two session refreshes must land while the
    // workspace is hidden — this is the path where a visibility-derived
    // enumeration would let the pool prune the claimed iframe.
    await page.waitForRequest(request => request.url().includes('/api/tmux/sessions'))
    await page.waitForRequest(request => request.url().includes('/api/tmux/sessions'))

    // Move around while hidden.
    await page.getByRole('button', { name: 'Terminal', exact: true }).click()
    await page.getByRole('button', { name: 'Settings' }).click()

    // Grow back and revisit.
    await page.getByRole('combobox', { name: 'Terminal tabs' }).selectOption('3')
    await page.getByRole('button', { name: 'Terminal 3' }).click()

    await expect(frameElement).toBeVisible()
    const survival = await frameElement.evaluate((el: HTMLIFrameElement) => ({
      tag: (el as HTMLIFrameElement & { __retentionTag?: string }).__retentionTag ?? null,
      marker: (el.contentWindow as (Window & { __retentionMarker?: string }) | null)?.__retentionMarker ?? null,
      src: el.src,
    }))
    expect(survival.tag).toBe('original-node')
    expect(survival.marker).toBe('alive')
    expect(survival.src).toBe(initialSrc)
    expect(survival.src).not.toContain('reconnect')

    // No reload and no reconnect across the whole shrink/grow cycle.
    expect(terminalDocumentLoads.filter(url => url.includes('arg=main'))).toHaveLength(baselineLoads)
  })

})
