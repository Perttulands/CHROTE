import { test, expect } from '../fixtures'
import { mockApiRoutes, mockFormationsApiRoutes, mockFormationsBoard } from '../mock-api'

test.describe('Formations Playwright stack smoke', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await mockFormationsApiRoutes(page)
  })

  test('keeps the Formations tab default-off', async ({ page }) => {
    await page.goto('/')

    await expect(page.getByRole('button', { name: 'Formations' })).toHaveCount(0)
    await expect(page.getByTestId('formations-view')).toHaveCount(0)
  })

  test('mounts mocked Formations data after explicit opt-in', async ({ page }) => {
    await page.addInitScript(() => {
      window.localStorage.setItem('chrote-formations', '1')
    })
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

  test('shows a usable starter canvas when the first-run board list is empty', async ({ page }) => {
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
    await page.addInitScript(() => {
      window.localStorage.setItem('chrote-formations', '1')
    })
    await page.goto('/')

    await page.getByRole('button', { name: 'Formations' }).click()

    await expect(page.getByTestId('formations-canvas')).toBeVisible()
    await expect(page.getByTestId('mission-node-mis_starter_session_search')).toContainText('Improve session search')
    await expect(page.getByTestId('formation-node-fmn_starter_frame')).toContainText('Frame the goal')
    await expect(page.getByTestId('formation-node-fmn_starter_research')).toContainText('Research huddle')
    await expect(page.getByTestId('gate-node-gate_starter_review')).toContainText('Review gate')
    await expect(page.getByTestId('formation-wire-edge_starter_mission_frame')).toBeVisible()

    await page.getByLabel('Formation title').fill('Quick huddle')
    await page.getByRole('button', { name: 'Peer' }).click()
    await expect(page.locator('[data-testid^="formation-node-fmn_starter_local_"]').filter({ hasText: 'Quick huddle' })).toBeVisible()
  })
})
