import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { openSessionsSidecar } from './helpers'

// The launcher is the only way CHROTE starts a session: in an empty window it
// is the window, and the Sessions plus opens the same panel.
test.describe('Launcher', () => {
  test('an empty window launches the chosen harness in the chosen folder and binds it', async ({ page }) => {
    await mockApiRoutes(page)

    const created: Array<Record<string, unknown>> = []
    page.on('request', request => {
      if (request.method() === 'POST' && request.url().includes('/api/tmux/sessions')) {
        created.push(JSON.parse(request.postData() || '{}'))
      }
    })

    await page.goto('/')
    await page.waitForSelector('.dashboard')

    const window = page.locator('.terminal-window').first()
    const launcher = window.locator('.launcher')
    await expect(launcher).toBeVisible()
    // No dashed New Session button survives anywhere in the empty window.
    await expect(window.locator('.tile-action-btn')).toHaveCount(0)
    await expect(window.locator('.empty-window-drop-hint')).toHaveText('or drag a session here')
    await expect(launcher.getByLabel('Session name')).toHaveValue('claude-chrote')

    await launcher.getByRole('button', { name: 'Launch claude in chrote' }).click()

    await expect.poll(() => created).toHaveLength(1)
    expect(created[0]).toMatchObject({
      name: 'claude-chrote',
      cwd: '/srv/chrote',
      harness: 'claude-code',
    })
    await expect(window.locator('.session-tag')).toHaveCount(1)
  })

  test('the Sessions plus opens the same launcher', async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page)

    await page.locator('.session-panel').getByTitle('New tmux session').click()

    const popup = page.locator('.session-launcher-popup')
    await expect(popup).toBeVisible()
    await expect(popup.locator('.launcher')).toBeVisible()
    await expect(popup.getByRole('button', { name: 'Open shell in chrote' })).toHaveCount(0)
    await popup.getByRole('button', { name: 'Shell' }).click()
    await expect(popup.getByRole('button', { name: 'Open shell in chrote' })).toBeVisible()
  })
})
