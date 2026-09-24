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
      body: JSON.stringify({ success: true, timestamp: '2026-09-16T00:00:00Z', data: {
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
      } }),
    })
  })
}

test.describe('what an agent sees', () => {
  test("a session's menu answers what the agent loaded, in three sections", async ({ page }) => {
    await page.setViewportSize({ width: 1400, height: 900 })
    await mockApiRoutes(page, { sessionsResponse: sessionsWithFolders })
    await mockAgentContextRoutes(page)
    let fileReads = 0
    await page.route('**/api/agent/file**', async route => {
      fileReads += 1
      await route.fulfill({ json: { success: true, data: {
        path: '/srv/chrote/CLAUDE.md', content: '# Instructions\n\nOriginal instructions.',
      } } })
    })
    await page.route('**/api/scheduled-tasks', async route => {
      await route.fulfill({ json: { success: true, data: { tasks: [] } } })
    })
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
    await page.locator('.terminal-workspace-dock[data-active="true"]')
      .getByRole('button', { name: 'Sessions sidecar', exact: true }).click()

    // The table moves between every kind of slot without replacing its nested
    // editor. Even a tab with no table only parks the work until it is wanted.
    await sheet.getByText('/srv/chrote/CLAUDE.md', { exact: true }).click()
    await expect(sheet).toContainText('Original instructions.')
    await sheet.getByRole('button', { name: 'Edit', exact: true }).click()
    const editor = sheet.getByRole('textbox', { name: 'Edit /srv/chrote/CLAUDE.md' })
    const draft = Array.from({ length: 100 }, (_, index) => `Unsaved instruction ${index}`).join('\n')
    await editor.fill(draft)
    const expansion = sheet.locator('.agent-expansion')
    const scrollTop = await expansion.evaluate(element => {
      element.scrollTop = 200
      return element.scrollTop
    })
    expect(scrollTop).toBeGreaterThan(0)

    for (const tab of ['Beads', 'Agents', 'Library', 'Scheduled']) {
      await page.getByRole('button', { name: tab, exact: true }).click()
      await expect(editor).toBeVisible()
      await expect(editor).toHaveValue(draft)
      expect(await expansion.evaluate(element => element.scrollTop)).toBe(scrollTop)
      await expect(page.locator('.table-column')).toHaveCount(1)
    }
    await page.getByRole('button', { name: 'Files', exact: true }).click()
    await expect(editor).toBeHidden()
    await page.keyboard.press('Alt+1')
    await expect(editor).toBeVisible()
    await expect(editor).toHaveValue(draft)
    expect(await expansion.evaluate(element => element.scrollTop)).toBe(scrollTop)
    expect(fileReads).toBe(1)

    // At phone width the same table overlays the active workspace and remains
    // dismissible from its own control; no new reader is created by the resize.
    await page.setViewportSize({ width: 390, height: 844 })
    await expect(sheet).toBeVisible()
    const [tableBox, workspaceBox] = await Promise.all([
      sheet.boundingBox(), page.locator('.terminal-workspace-dock[data-active="true"]').boundingBox(),
    ])
    expect(tableBox).not.toBeNull()
    expect(workspaceBox).not.toBeNull()
    expect(Math.round(tableBox!.width)).toBe(Math.round(workspaceBox!.width))
    expect(Math.round(tableBox!.x)).toBe(Math.round(workspaceBox!.x))
    await sheet.getByRole('button', { name: 'Close', exact: true }).click()
    await expect(sheet).toHaveCount(0)
  })
})
