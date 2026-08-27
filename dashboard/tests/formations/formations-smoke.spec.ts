import { test, expect } from '../fixtures'
import { mockApiRoutes, mockCodeGateProfiles, mockFormationsApiRoutes, mockFormationsBoard } from '../mock-api'

test.describe('Formations Playwright stack smoke', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await mockFormationsApiRoutes(page)
  })

  test('shows the Formations tab by default with no opt-in', async ({ page }) => {
    await page.goto('/')

    // Formations is always-on: the tab is present with no localStorage tinkering.
    await expect(page.getByRole('button', { name: 'Formations' })).toBeVisible()
  })

  test('mounts mocked Formations data on the default tab', async ({ page }) => {
    await page.goto('/')

    await page.getByRole('button', { name: 'Formations' }).click()

    await expect(page.getByTestId('formations-view')).toBeVisible()
    await expect(page.getByTestId(`formation-node-${mockFormationsBoard.formations[0].id}`))
      .toContainText(mockFormationsBoard.formations[0].title)
    await expect(page.getByTestId(`mission-node-${mockFormationsBoard.missions[0].id}`))
      .toContainText(mockFormationsBoard.missions[0].title)
    await expect(page.getByTestId(`gate-node-${mockFormationsBoard.gates[0].id}`))
      .toContainText(mockFormationsBoard.gates[0].title)
  })

  test('shares board and element notes through the collapsible right rail', async ({ page }) => {
    await page.goto('/')
    await page.getByRole('button', { name: 'Formations' }).click()

    const notepad = page.getByRole('complementary', { name: 'Shared board notepad' })
    await expect(notepad.getByRole('button', { name: 'Expand shared notepad' })).toBeVisible()
    await notepad.getByRole('button', { name: 'Expand shared notepad' }).click()
    await notepad.getByRole('textbox', { name: 'Board note' }).fill('Use the board sketch as implementation context.')
    await notepad.getByRole('button', { name: 'Save board note' }).click()
    await expect(notepad).toContainText('rev 1')

    const formation = mockFormationsBoard.formations[0]
    await page.getByRole('button', { name: `Add note for ${formation.title}` }).click()
    await notepad.getByRole('textbox', { name: 'Element note' }).fill('Builder owns this step.')
    await notepad.getByRole('button', { name: 'Save element note' }).click()
    await expect(page.getByTestId(`formation-node-${formation.id}`)).toHaveClass(/has-note/)

    await notepad.getByRole('button', { name: 'Collapse shared notepad' }).click()
    await expect(notepad.getByRole('button', { name: 'Expand shared notepad' })).toBeVisible()
  })

  test('groups the complete Codex role preset set beside Claude agents', async ({ page }) => {
    const roles = ['Scout', 'Planner', 'Builder', 'Judge', 'Orchestrator', 'Debugger', 'Reviewer']
    await page.route('**/api/agents**', async route => {
      const agents = [
        { id: 'claude-builder', displayName: 'Claude Builder', kind: 'builder', harnessDefault: 'claude-code', assignable: true, liveness: 'offline' },
        ...roles.map(role => ({
          id: `codex-${role.toLowerCase()}`,
          displayName: `Codex ${role}`,
          kind: role.toLowerCase(),
          harnessDefault: 'openai-codex',
          assignable: true,
          liveness: 'offline',
          preset: true,
        })),
      ]
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, data: { agents, count: agents.length } }),
      })
    })
    await page.goto('/')
    await page.getByRole('button', { name: 'Formations' }).click()

    const roster = page.getByRole('complementary', { name: 'Agent roster' })
    await expect(roster.getByText('Codex', { exact: true })).toBeVisible()
    await expect(roster.getByText('Claude', { exact: true })).toBeVisible()
    for (const role of roles) await expect(roster.getByText(`Codex ${role}`)).toBeVisible()
  })

  test('fails loud instead of fabricating a starter canvas when the first-run board list is empty', async ({ page }) => {
    await mockApiRoutes(page)
    const createdBoard = {
      ...mockFormationsBoard,
      id: 'board-release-sketch',
      slug: 'release-sketch',
      title: 'Release sketch',
      rev: 1,
      etag: 'release-sketch-etag',
      missions: [],
      formations: [],
      gates: [],
      tools: [],
      connections: [],
    }
    await page.route('**/api/agents**', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          timestamp: new Date().toISOString(),
          data: { agents: [] },
        }),
      })
    })
    await page.route('**/api/formations/**', async route => {
      const request = route.request()
      const path = new URL(request.url()).pathname
      if (request.method() === 'GET' && path === '/api/formations/gate-profiles') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            success: true,
            timestamp: new Date().toISOString(),
            data: { profiles: mockCodeGateProfiles },
          }),
        })
        return
      }
      if (request.method() === 'GET' && path === '/api/formations/boards') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            success: true,
            timestamp: new Date().toISOString(),
            data: { boards: [] },
          }),
        })
        return
      }
      if (request.method() === 'POST' && path === '/api/formations/boards') {
        await route.fulfill({
          status: 201,
          contentType: 'application/json',
          headers: { ETag: createdBoard.etag },
          body: JSON.stringify({ success: true, data: { board: createdBoard } }),
        })
        return
      }
      if (request.method() === 'GET' && path === `/api/formations/boards/${createdBoard.slug}`) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          headers: { ETag: createdBoard.etag },
          body: JSON.stringify({ success: true, data: { board: createdBoard } }),
        })
        return
      }
      if (request.method() === 'GET' && path === `/api/formations/boards/${createdBoard.slug}/layout`) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          headers: { ETag: '*' },
          body: JSON.stringify({ success: true, data: { layout: { schema: 1, boardId: createdBoard.id, boardRev: createdBoard.rev, nodes: [], edges: [], etag: '*' } } }),
        })
        return
      }
      if (request.method() === 'GET' && path === `/api/formations/boards/${createdBoard.slug}/notes`) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          headers: { ETag: '*' },
          body: JSON.stringify({ success: true, data: { notes: { schema: 1, boardId: createdBoard.id, rev: 0, board: '', elements: [], etag: '*' } } }),
        })
        return
      }
      if (request.method() === 'GET' && path === `/api/formations/boards/${createdBoard.slug}/changes`) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true, data: { signal: { board: createdBoard.slug, changed: false, rev: createdBoard.rev, etag: createdBoard.etag } } }),
        })
        return
      }
      await route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({
          success: false,
          error: { code: 'MOCK_NOT_FOUND', message: `${request.method()} ${path}` },
        }),
      })
    })
    await page.goto('/')

    await page.getByRole('button', { name: 'Formations' }).click()

    await expect(page.locator('.fmx[data-cockpit="d7"]')).toBeVisible()
    await expect(page.getByTestId('formations-canvas')).toBeVisible()
    await expect(page.getByTestId('formations-empty-board')).toContainText('No persisted formation boards')
    await expect(page.getByTestId('formations-empty-board')).toContainText('Create a board from the top bar')
    await expect(page.getByTestId('mission-node-mis_starter_session_search')).toHaveCount(0)
    await expect(page.getByTestId('formation-node-fmn_starter_frame')).toHaveCount(0)
    await expect(page.getByTestId('gate-node-gate_starter_review')).toHaveCount(0)
    await expect(page.getByTestId('new-formation')).toBeDisabled()
    await expect(page.getByTestId('new-board')).toBeEnabled()

    await page.getByTestId('new-board').click()
    const dialog = page.getByRole('dialog', { name: 'Create board' })
    await dialog.getByLabel('Board name').fill('Release sketch')
    await dialog.getByRole('button', { name: 'Create board' }).click()
    await expect(page.getByTestId('board-picker')).toHaveValue('release-sketch')
    await expect(page.getByTestId('formations-empty-board')).toContainText('This board is empty')
    await expect(page.getByTestId('new-formation')).toBeEnabled()
  })
})
