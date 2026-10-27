import { test, expect } from './fixtures'
import { mockApiRoutes, mockResidentsApiRoute } from './mock-api'

test('services open drawn, start and stop by their buttons, and hand the chosen row to the resident', async ({ page }) => {
  await mockApiRoutes(page)
  await mockResidentsApiRoute(page, [{ tab: 'services', label: 'Service agent', session: 'hq-deacon', folder: '/work/example', beads: '' }])
  let running = false
  let reads = 0
  await page.route('**/api/services/**', async route => {
    running = route.request().url().endsWith('/start')
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ errors: [] }) })
  })
  await page.route('**/api/services', async route => {
    reads += 1
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ services: [{ id: 'example', name: 'Example', description: 'Example service', link: 'https://example.test/', state: running ? 'running' : 'stopped', controllable: true }] }) })
  })
  await page.goto('/')
  const tab = page.getByRole('button', { name: 'Services', exact: true })
  // The pointer on its way to the tab asks for the list, so the click opens on it.
  await tab.hover()
  await expect.poll(() => reads).toBe(1)
  await tab.click()
  const row = page.getByRole('listitem', { name: /Example/ })
  await expect(row).toContainText('stopped')
  await row.getByRole('button', { name: /Start/ }).click()
  await expect(row).toContainText('running')
  await row.getByRole('button', { name: /Stop/ }).click()
  await expect(row).toContainText('stopped')
  await expect(row.getByRole('button', { name: /Start/ })).toBeVisible()
  const agent = page.getByRole('complementary', { name: 'The Service agent' })
  await agent.getByRole('button', { name: 'Expand the Service agent' }).click()
  await expect(agent.locator('.xterm-rows')).toContainText('mock terminal hq-deacon')
})

test('phone keeps controls and an action failure visible without a configured agent', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mockApiRoutes(page)
  await mockResidentsApiRoute(page, [{ tab: 'services', label: 'Service agent', session: '', folder: '', beads: '' }])
  await page.route('**/api/services/**', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ errors: ['Could not start the service'] }) }))
  await page.route('**/api/services', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ services: [{ id: 'example', name: 'Example', description: 'Example service', link: '', state: 'stopped', controllable: true }] }) }))
  await page.goto('/')
  await page.getByRole('button', { name: '☰' }).click()
  await page.getByRole('button', { name: 'Services', exact: true }).click()
  const row = page.getByRole('listitem', { name: /Example/ })
  const name = row.getByText('Example', { exact: true })
  const start = row.getByRole('button', { name: 'Start' })
  await expect(name).toBeVisible()
  const nameBox = await name.boundingBox()
  const startBox = await start.boundingBox()
  expect(nameBox!.width).toBeGreaterThan(70)
  expect(startBox!.x + startBox!.width).toBeLessThanOrEqual(390)
  await start.click()
  await expect(row.getByRole('alert')).toHaveText('Could not start the service')
  await expect(row.getByRole('alert')).toBeVisible()
})

test('keeps service actions visible when another panel narrows the desktop pane', async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 768 })
  await mockApiRoutes(page)
  await mockResidentsApiRoute(page, [{ tab: 'services', label: 'Service agent', session: 'hq-deacon', folder: '/work/example', beads: '' }])
  await page.route('**/api/services', route => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ services: [{ id: 'example', name: 'Example', description: 'Example service', link: '', state: 'stopped', controllable: true }] }) }))
  await page.goto('/')
  await page.getByRole('button', { name: 'Services', exact: true }).click()
  const main = page.locator('.services-main')
  await main.evaluate(element => { element.style.flex = 'none'; element.style.width = '480px' })
  const row = page.getByRole('listitem', { name: /Example/ })
  const [mainBox, startBox] = [await main.boundingBox(), await row.getByRole('button', { name: 'Start' }).boundingBox()]
  expect(startBox!.x + startBox!.width).toBeLessThanOrEqual(mainBox!.x + mainBox!.width + 1)
  await expect(row.getByText('Example service')).toBeHidden()
})
