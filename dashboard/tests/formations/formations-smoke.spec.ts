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
})
