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

  test('fails loud instead of fabricating a starter canvas when the first-run board list is empty', async ({ page }) => {
    await mockApiRoutes(page)
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
    await expect(page.getByTestId('formations-empty-board')).toContainText('no longer shows fake starter missions')
    await expect(page.getByTestId('mission-node-mis_starter_session_search')).toHaveCount(0)
    await expect(page.getByTestId('formation-node-fmn_starter_frame')).toHaveCount(0)
    await expect(page.getByTestId('gate-node-gate_starter_review')).toHaveCount(0)
    await expect(page.getByTestId('new-formation')).toBeDisabled()
  })
})
