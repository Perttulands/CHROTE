import { test, expect, Page } from './fixtures'
import { mockBeadsProjectsRoute, mockFileApiRoutes, mockLaunchApiRoute, mockPersistentTabApiRoutes, mockSessions, mockTerminalSocket, mockThemeApiRoute, mockWorkspacesRoute } from './mock-api'
import { dragAndDrop, openSessionsSidecar } from './helpers'

/**
 * A rename has to reach every surface showing the session at once — the row it
 * was renamed from, and the tag on the tile the operator is watching — and the
 * kill that follows has to arm before it runs. One journey, because the cost of
 * mounting the dashboard dwarfs the cost of any assertion in it.
 */

interface Rename { from: string; to: string }
interface Mutations { renames: Rename[]; deletes: string[]; copies: Record<string, unknown>[] }

/**
 * API mocks that also answer DELETE and PATCH for sessions. A delete removes
 * the session from later GETs and a rename replaces the name in them, so the
 * poll tells the dashboard what the mutation really did.
 */
async function mockApiRoutesWithMutations(page: Page): Promise<Mutations> {
  await mockTerminalSocket(page)

  await mockThemeApiRoute(page)
  await mockLaunchApiRoute(page)

  await mockFileApiRoutes(page)
  // A terminal asks which Beads projects exist, to link the ids in its output;
  // the workspace list carries them, and the launcher asks for it too.
  await mockWorkspacesRoute(page)
  await mockBeadsProjectsRoute(page)
  await mockPersistentTabApiRoutes(page)

  const renames: Rename[] = []
  const deletes: string[] = []
  const copies: Record<string, unknown>[] = []

  // Mutable copy of session list so delete/rename are reflected on refresh.
  // Two sessions belong to Unix users, so a kill has to say whose it is.
  let sessions: Array<(typeof mockSessions.sessions)[number] & { unixUser?: string }> = structuredClone(mockSessions.sessions)
    .map(s => s.name === 'gt-gastown-jack' ? { ...s, unixUser: 'alice', cwd: '/code/project', currentCommand: 'codex' } : s.name === 'gt-gastown-joe' ? { ...s, unixUser: 'bob', cwd: '/code/project', currentCommand: 'claude' } : s)

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
    } else if (request.method() === 'POST') {
      const body = request.postDataJSON() as Record<string, unknown>
      copies.push(body)
      if (sessions.some(session => session.name === body.name && (session.unixUser ?? '') === body.unixUser)) {
        await route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: { code: 'SESSION_NAME_CONFLICT' } }) })
        return
      }
      const original = sessions.find(session => (session.unixUser ?? '') === body.unixUser)
      if (!original) throw new Error('Copy requested an unknown Unix user')
      sessions.push({ ...original, name: String(body.name), attached: false, cwd: String(body.cwd) })
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ success: true }) })
    } else {
      await route.continue()
    }
  })

  // DELETE /api/tmux/sessions/<name>
  await page.route('**/api/tmux/sessions/*', async (route, request) => {
    const url = request.url()
    const encodedName = url.split('/api/tmux/sessions/')[1]?.split('?')[0]
    if (!encodedName) { await route.continue(); return }
    const sessionName = decodeURIComponent(encodedName)

    if (request.method() === 'DELETE') {
      const { pathname, search } = new URL(url)
      deletes.push(decodeURIComponent(pathname.replace('/api/tmux/sessions/', '')) + search)
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

  return { renames, deletes, copies }
}

test.describe('Session Context Menu', () => {
  let renames: Rename[]
  let deletes: string[]
  let copies: Record<string, unknown>[]

  test.beforeEach(async ({ page }) => {
    ;({ renames, deletes, copies } = await mockApiRoutesWithMutations(page))
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page)
    await page.waitForSelector('.session-item')
  })

  test('copies Codex and Claude sessions from the menu with sequential names in their folder', async ({ page }) => {
    const row = (name: string) => page.locator('.session-panel .session-item').filter({ has: page.locator(`.session-name[title="${name}"]`) })
    for (const [name, harness, unixUser] of [['gt-gastown-jack', 'codex', 'alice'], ['gt-gastown-joe', 'claude-code', 'bob']]) {
      for (const suffix of [2, 3, 4]) {
        await row(name).click({ button: 'right' })
        await page.getByRole('menuitem', { name: 'Copy session', exact: true }).click()
        await expect(row(`${name}-${suffix}`)).toBeVisible()
        expect(copies.at(-1)).toMatchObject({ name: `${name}-${suffix}`, cwd: '/code/project', harness, unixUser })
        await expect(row(name)).toBeVisible()
      }
    }
    expect(deletes).toEqual([])
    expect(renames).toEqual([])
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

  test('kills a Ctrl and Shift selection through the menu and through Delete, never a hidden row', async ({ page }) => {
    const panel = page.locator('.session-panel')
    const row = (name: string) => panel.locator(`.session-item:has(.session-name[title="${name}"])`)

    // Ctrl picks one row; Shift sweeps from it to another; Ctrl adds a row
    // from another group.
    await row('gt-gastown-jack').click({ modifiers: ['ControlOrMeta'] })
    await row('gt-gastown-max').click({ modifiers: ['Shift'] })
    await row('hq-mayor').click({ modifiers: ['ControlOrMeta'] })

    // Filtered out, hq-mayor stays selected but is not one of the kills.
    await page.getByPlaceholder('Filter sessions...').fill('gastown')
    await expect(row('hq-mayor')).toHaveCount(0)

    await row('gt-gastown-joe').click({ button: 'right' })
    await page.getByRole('menuitem', { name: 'Kill 3 sessions' }).click()
    await page.getByRole('menuitem', { name: 'Confirm kill 3' }).click()

    await expect.poll(() => [...deletes].sort()).toEqual([
      'gt-gastown-jack?unixUser=alice',
      'gt-gastown-joe?unixUser=bob',
      'gt-gastown-max',
    ])
    await expect(row('gt-gastown-max')).toHaveCount(0)

    // Back in view, hq-mayor is still selected; Ctrl adds main, and Delete
    // asks for the same confirm before anything is killed.
    await page.getByPlaceholder('Filter sessions...').fill('')
    await row('main').click({ modifiers: ['ControlOrMeta'] })
    await page.keyboard.press('Delete')
    await expect(page.getByRole('menuitem', { name: 'Confirm kill 2' })).toBeFocused()
    expect(deletes).toHaveLength(3)
    await page.keyboard.press('Enter')

    await expect.poll(() => deletes.slice(3).sort()).toEqual(['hq-mayor', 'main'])
    await expect(row('hq-deacon')).toBeVisible()
    expect(deletes).toHaveLength(5)
  })
})
