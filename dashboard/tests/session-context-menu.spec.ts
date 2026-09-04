import { test, expect, Page } from './fixtures'
import { mockBeadsProjectsRoute, mockFileApiRoutes, mockLaunchApiRoute, mockSessions, mockTerminalSocket, mockThemeApiRoute, mockWorkspacesRoute } from './mock-api'
import { dragAndDrop, openSessionsSidecar } from './helpers'

/**
 * A rename has to reach every surface showing the session at once — the row it
 * was renamed from, and the tag on the tile the operator is watching — and the
 * kill that follows has to arm before it runs. One journey, because the cost of
 * mounting the dashboard dwarfs the cost of any assertion in it.
 */

interface Rename { from: string; to: string }

/**
 * API mocks that also answer DELETE and PATCH for sessions. A delete removes
 * the session from later GETs and a rename replaces the name in them, so the
 * poll tells the dashboard what the mutation really did.
 */
async function mockApiRoutesWithMutations(page: Page): Promise<Rename[]> {
  await mockTerminalSocket(page)

  await mockThemeApiRoute(page)
  await mockLaunchApiRoute(page)

  await mockFileApiRoutes(page)
  // A terminal asks which Beads projects exist, to link the ids in its output;
  // the workspace list carries them, and the launcher asks for it too.
  await mockWorkspacesRoute(page)
  await mockBeadsProjectsRoute(page)

  const renames: Rename[] = []

  // Mutable copy of session list so delete/rename are reflected on refresh
  let sessions = structuredClone(mockSessions.sessions)

  const buildResponse = () => {
    const grouped: Record<string, typeof sessions> = {}
    for (const s of sessions) {
      const g = s.group
      if (!grouped[g]) grouped[g] = []
      grouped[g].push(s)
    }
    return {
      sessions,
      grouped,
      timestamp: new Date().toISOString(),
    }
  }

  // GET /api/tmux/sessions
  await page.route('**/api/tmux/sessions', async (route, request) => {
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(buildResponse()),
      })
    } else {
      await route.continue()
    }
  })

  // DELETE /api/tmux/sessions/<name>
  await page.route('**/api/tmux/sessions/*', async (route, request) => {
    const url = request.url()
    const encodedName = url.split('/api/tmux/sessions/')[1]
    if (!encodedName) { await route.continue(); return }
    const sessionName = decodeURIComponent(encodedName)

    if (request.method() === 'DELETE') {
      sessions = sessions.filter(s => s.name !== sessionName)
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true }) })
    } else if (request.method() === 'PATCH') {
      const body = request.postDataJSON()
      const newName = body?.newName
      if (newName) {
        renames.push({ from: sessionName, to: newName })
        const idx = sessions.findIndex(s => s.name === sessionName)
        if (idx !== -1) sessions[idx] = { ...sessions[idx], name: newName }
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true }) })
    } else {
      await route.continue()
    }
  })

  return renames
}

test.describe('Session Context Menu', () => {
  let renames: Rename[]

  test.beforeEach(async ({ page }) => {
    renames = await mockApiRoutesWithMutations(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page)
    await page.waitForSelector('.session-item')
  })

  test('renames an attached session from the row and from its tag, then kills it', async ({ page }) => {
    const window = page.locator('.terminal-window').first()

    await dragAndDrop(page, '.session-panel .session-item:has-text("hq-mayor")', '.terminal-window')
    await expect(window.locator('.tag-name')).toContainText('hq-mayor')

    // Renaming from the row has to reach the tag on the tile as well as the row.
    await page.locator('.session-panel .session-item:has-text("hq-mayor")').click({ button: 'right' })
    await expect(page.locator('.menu-sheet')).toBeVisible()
    await page.locator('.menu-row:has-text("Rename")').click()
    const rowInput = page.locator('.session-rename-input')
    await expect(rowInput).toBeVisible()
    await rowInput.fill('hq-commander')
    await rowInput.press('Enter')

    await expect.poll(() => renames).toEqual([{ from: 'hq-mayor', to: 'hq-commander' }])
    await expect(window.locator('.tag-name')).toContainText('hq-commander')
    await expect(page.locator('.session-item:has-text("hq-commander")')).toBeVisible()

    // The tag's own menu offers the same rename, and its field has to keep the
    // cursor through the mount that puts it there.
    const tag = window.locator('.session-tag:has-text("hq-commander")')
    await tag.click({ button: 'right' })
    const menu = page.getByRole('menu', { name: 'Session actions for hq-commander' })
    await menu.getByRole('menuitem', { name: 'Rename session' }).click()

    const tagInput = window.getByRole('textbox', { name: 'Rename session hq-commander' })
    await expect(tagInput).toBeFocused()
    await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
    await expect(tagInput).toBeFocused()
    await expect(tagInput).toHaveValue('hq-commander')
    await tagInput.fill('hq-marshal')
    await tagInput.press('Enter')

    await expect(window.locator('.session-tag:has-text("hq-marshal")')).toBeVisible()
    await expect(window.locator('.session-tag:has-text("hq-commander")')).not.toBeVisible()

    // The header is the session it shows: a right-click on its empty stretch,
    // away from the tag, opens the same menu the tag does.
    const header = window.locator('.terminal-window-header')
    const headerBox = await header.boundingBox()
    if (!headerBox) throw new Error('the header has no bounding box')
    await header.click({ button: 'right', position: { x: headerBox.width / 2, y: headerBox.height / 2 } })
    const headerMenu = page.getByRole('menu', { name: 'Session actions for hq-marshal' })
    await expect(headerMenu).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(headerMenu).toHaveCount(0)

    // Kill confirms where it was chosen: the row arms, then it runs.
    await page.locator('.session-panel .session-item:has-text("hq-marshal")').click({ button: 'right' })
    await page.locator('.menu-sheet .menu-row:has-text("Kill session")').click()
    await page.locator('.menu-sheet .menu-row:has-text("Confirm kill")').click()

    await expect(page.locator('.session-item:has-text("hq-marshal")')).not.toBeVisible()
  })
})
