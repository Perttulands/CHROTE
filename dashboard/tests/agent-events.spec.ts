import { test, expect, type Page } from './fixtures'
import { openSessionsSidecar } from './helpers'
import { mockAgentEventSeenRoute, mockApiRoutes, sessionsWithLastEvent } from './mock-api'
import type { AgentEvent, SessionsResponse } from '../src/types'

/**
 * Agent events (bead chrote-5grx.57).
 *
 * An agent's own completion hook tells the server; the session list carries
 * the report. On this device the report marks the session's row and the tab
 * holding it, and the toast names it, until the tile showing the session is
 * focused, which tells the server it was seen. A report that was already in
 * the list when the page loaded is history and marks nothing.
 */

const ARCHITECT = 'claude-chrote-architect'
const BUILDER = 'claude-chrote-builder'
const USER = 'chrote'
const key = (name: string) => `${USER}:${name}`

const sessions = [
  { name: ARCHITECT, windows: 1, attached: false, group: 'claude', unixUser: USER, cwd: '/srv/chrote', currentCommand: 'claude' },
  { name: BUILDER, windows: 1, attached: false, group: 'claude', unixUser: USER, cwd: '/srv/chrote', currentCommand: 'claude' },
]

const plain: SessionsResponse = { sessions, grouped: { claude: sessions }, timestamp: new Date().toISOString() }

/** The architect in the first tab, the builder in the second; nothing focused. */
function seededState() {
  const window = (id: string, session: string | null) => ({
    id, boundSessions: session ? [session] : [], activeSession: session, colorIndex: 0,
  })
  return {
    workspaces: {
      terminal1: { windowCount: 1, windows: [window('terminal1-window-0', key(ARCHITECT))] },
      terminal2: { windowCount: 1, windows: [window('terminal2-window-0', key(BUILDER))] },
      terminal3: { windowCount: 1, windows: [window('terminal3-window-0', null)] },
    },
    sidebarCollapsed: false,
    settings: { fontSize: 14, autoRefreshInterval: 500 },
  }
}

/** The session list the poll answers with, changed under the page mid-journey. */
async function open(page: Page) {
  const list = { current: plain, polls: 0 }
  await mockApiRoutes(page, { sessionsResponse: plain })
  await page.route(/.*\/api\/tmux\/sessions\/?$/, async route => {
    if (route.request().method() !== 'GET') {
      await route.fallback()
      return
    }
    list.polls += 1
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(list.current) })
  })
  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, seededState())
  await page.goto('/')
  return list
}

test.describe('Agent events', () => {
  test('a report marks its row and its tab and names itself in the toast, until the tile is focused', async ({ page }) => {
    const seen = await mockAgentEventSeenRoute(page)
    const list = await open(page)
    await openSessionsSidecar(page)

    const row = page.locator('.session-item').filter({ hasText: BUILDER })
    const rowMark = row.locator('.session-event-mark')
    const tab = page.getByRole('button', { name: 'Terminal 2', exact: true })
    const tabMark = tab.locator('.tab-event-mark')
    await expect(row).toBeVisible()
    await expect(rowMark).toHaveCount(0)
    await expect(tabMark).not.toHaveClass(/on/)

    // The first list has landed and is history. Then the builder's hook fires.
    await expect.poll(() => list.polls).toBeGreaterThan(0)
    const report: AgentEvent = { event: 'finished', time: new Date().toISOString(), summary: 'Wrote the tests; all green', seen: false }
    list.current = sessionsWithLastEvent(plain, BUILDER, report)

    await expect(rowMark).toBeVisible()
    await expect(page.locator('.toast')).toHaveText(`${BUILDER} finished`)
    await expect(row.locator('.session-event-summary')).toHaveText('Wrote the tests; all green')
    await expect(tabMark).toHaveClass(/on/)
    // The architect's row and tab say nothing: the report is the builder's.
    await expect(page.locator('.session-item').filter({ hasText: ARCHITECT }).locator('.session-event-mark')).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Terminal', exact: true }).locator('.tab-event-mark')).not.toHaveClass(/on/)
    expect(seen).toEqual([])

    // Focusing the tile that shows the session is seeing it: both marks go at
    // once, and the server is told which session, under which user.
    await tab.click()
    await page.locator('.terminal-grid[data-workspace="terminal2"] .terminal-window-body').first().click()

    await expect(rowMark).toHaveCount(0)
    await expect(tabMark).not.toHaveClass(/on/)
    await expect.poll(() => seen).toEqual([{ session: BUILDER, unixUser: USER }])

    // The server agrees on the next poll, and nothing comes back.
    list.current = sessionsWithLastEvent(plain, BUILDER, { ...report, seen: true })
    const polled = list.polls
    await expect.poll(() => list.polls).toBeGreaterThan(polled)
    await expect(rowMark).toHaveCount(0)
    expect(seen).toHaveLength(1)
  })
})
