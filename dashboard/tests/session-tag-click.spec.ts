import { test, expect, type Locator, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Switching sessions from the tag (bead chrote-xz2y).
 *
 * The tag is both the switch between the sessions bound to one window and the
 * drag surface that moves a binding elsewhere, so an unsteady press — the
 * ordinary click on a trackpad or a touchscreen — must still switch. Past the
 * sensor's 8px threshold the press becomes a drag, and dnd-kit swallows the
 * click that would have followed; dropping a tag back on its own window is a
 * no-op, so nothing at all used to happen.
 */

const SHOWN = 'claude-chrote-architect'
const OTHER = 'claude-chrote-builder'
const USER = 'chrote'
const shownKey = `${USER}:${SHOWN}`
const otherKey = `${USER}:${OTHER}`

const sessions = [
  { name: SHOWN, windows: 1, attached: false, group: 'claude', unixUser: USER, cwd: '/srv/chrote', currentCommand: 'claude' },
  { name: OTHER, windows: 1, attached: false, group: 'claude', unixUser: USER, cwd: '/srv/chrote', currentCommand: 'codex' },
]

function seededState() {
  return {
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [
          {
            id: 'terminal1-window-0',
            boundSessions: [shownKey, otherKey],
            activeSession: shownKey,
            colorIndex: 0,
          },
        ],
      },
      terminal2: { windowCount: 1, windows: [] },
      terminal3: { windowCount: 1, windows: [] },
    },
    sidebarCollapsed: true,
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
  }
}

async function openWindow(page: Page) {
  await mockApiRoutes(page, {
    sessionsResponse: { sessions, grouped: { claude: sessions }, timestamp: new Date().toISOString() },
  })
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState())
  await page.goto('/')
  const window = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window').first()
  await expect(window.locator('.session-tag')).toHaveCount(2)
  return window
}

const tagFor = (window: Locator, name: string) => window.locator('.session-tag').filter({ hasText: name })

/** A press that drifts sideways before it lifts, the way a real click does. */
async function unsteadyClick(page: Page, tag: Locator) {
  const box = await tag.boundingBox()
  if (!box) throw new Error('the tag has no bounding box')
  const x = box.x + box.width / 2
  const y = box.y + box.height / 2
  await page.mouse.move(x, y)
  await page.mouse.down()
  await page.mouse.move(x + 12, y + 2, { steps: 3 })
  await page.mouse.up()
}

test.describe('Session tag click', () => {
  // Both presses on one page: the steady click a mouse makes, and the drifting
  // one a trackpad or a touchscreen makes, which crosses dnd-kit's threshold.
  test('shows the clicked session and marks its tag active, however unsteady the press', async ({ page }) => {
    const window = await openWindow(page)
    const other = tagFor(window, OTHER)
    const shown = tagFor(window, SHOWN)
    const surface = window.locator('.terminal-surface-host:visible .xterm-rows')
    await expect(other).not.toHaveClass(/active/)

    await other.click()

    await expect(other).toHaveClass(/active/)
    await expect(shown).not.toHaveClass(/active/)
    await expect(surface).toContainText(`mock terminal ${OTHER}`)

    // Back to the first, so the drifting press has somewhere to switch to.
    await shown.click()
    await expect(shown).toHaveClass(/active/)
    await expect(surface).toContainText(`mock terminal ${SHOWN}`)

    await unsteadyClick(page, other)

    await expect(other).toHaveClass(/active/)
    await expect(surface).toContainText(`mock terminal ${OTHER}`)
  })
})
