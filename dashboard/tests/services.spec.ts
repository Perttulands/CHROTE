import { test, expect } from './fixtures'
import { mockApiRoutes, mockResidentsApiRoute } from './mock-api'

test('services list, toggle and resident drawer use configured data', async ({ page }) => {
  await mockApiRoutes(page)
  await mockResidentsApiRoute(page, [{ tab: 'services', label: 'Service agent', session: 'hq-deacon', folder: '/work/example', beads: '' }])
  let running = false
  await page.route('**/api/services/**', async route => {
    running = route.request().url().endsWith('/start')
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ errors: [] }) })
  })
  await page.route('**/api/services', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ services: [{ id: 'example', name: 'Example', description: 'Example service', link: 'https://example.test/', state: running ? 'running' : 'stopped', controllable: true }] }) })
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Services', exact: true }).click()
  await expect(page.getByText('Example service')).toBeVisible()
  const toggle = page.getByRole('checkbox', { name: 'Example running' })
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'true')
  await toggle.click()
  await expect(toggle).toHaveAttribute('aria-checked', 'false')
  const agent = page.getByRole('complementary', { name: 'The Service agent' })
  await agent.getByRole('button', { name: 'Expand the Service agent' }).click()
  await expect(agent.locator('.xterm-rows')).toContainText('mock terminal hq-deacon')
})
