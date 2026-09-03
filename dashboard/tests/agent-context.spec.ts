import { expect, test, type Page } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'
import { openSessionsSidecar } from './helpers'

/**
 * Journey 4: understand what an agent sees.
 *
 * The browser is the point here — the way in is a session's own menu, and the
 * answer goes on the table, in the column beside the tiles rather than over
 * them. What the stack contains is a unit test's business; what this proves is
 * that the question can be asked of a running agent and answered in one step.
 */

const AGENT = 'gt-gastown-jack'

/** The same list, with the agent's session reporting where and what it runs. */
function asAgent<T extends { name: string }>(session: T) {
  return session.name === AGENT ? { ...session, cwd: '/srv/chrote', currentCommand: 'claude' } : session
}

const sessionsWithFolders = {
  ...mockSessions,
  sessions: mockSessions.sessions.map(asAgent),
  grouped: Object.fromEntries(
    Object.entries(mockSessions.grouped).map(([group, members]) => [group, members.map(asAgent)]),
  ),
}

async function mockAgentContextRoutes(page: Page) {
  await page.route('**/api/agent/context**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        folder: '/srv/chrote',
        harness: 'claude-code',
        user: '',
        instructions: [
          { path: '/srv/chrote/CLAUDE.md', scope: 'project', kind: 'CLAUDE.md', readable: true, size: 3709 },
          { path: '/home/operator/.claude/settings.json', scope: 'user', kind: 'settings', readable: false, size: 0 },
        ],
        skills: [
          {
            name: 'dashboard-development',
            description: 'Change CHROTE dashboard views.',
            path: '/srv/chrote/skills/dashboard-development',
            source: 'project',
          },
        ],
        memories: [
          {
            kind: 'claude-auto',
            path: '/home/operator/.claude/projects/-srv-chrote/memory/MEMORY.md',
            title: 'MEMORY.md',
            updated: '2026-09-03T15:31:00Z',
            readable: true,
          },
        ],
      }),
    })
  })
}

test.describe('what an agent sees', () => {
  test("a session's menu answers what the agent loaded, in three sections", async ({ page }) => {
    await page.setViewportSize({ width: 1400, height: 900 })
    await mockApiRoutes(page, { sessionsResponse: sessionsWithFolders })
    await mockAgentContextRoutes(page)
    await page.goto('/')

    await openSessionsSidecar(page)
    await page.getByRole('button', { name: `Session actions for ${AGENT}` }).click()
    await page.getByText('What this agent sees').click()

    const sheet = page.getByRole('complementary', { name: `What ${AGENT} sees` })
    await expect(sheet).toBeVisible()
    await expect(sheet.getByRole('heading', { name: 'Instructions' })).toBeVisible()
    await expect(sheet.getByRole('heading', { name: 'Skills' })).toBeVisible()
    await expect(sheet.getByRole('heading', { name: 'Memories' })).toBeVisible()
    await expect(sheet.getByText('/srv/chrote/CLAUDE.md')).toBeVisible()
    await expect(sheet.getByText('dashboard-development')).toBeVisible()
    await expect(sheet.getByText('MEMORY.md')).toBeVisible()
    await expect(sheet.getByText('not readable by the server')).toBeVisible()
  })
})
