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

  test('the flags catalogue writes the line the session is launched with', async ({ page }) => {
    await mockApiRoutes(page)

    const created: Array<Record<string, unknown>> = []
    page.on('request', request => {
      if (request.method() === 'POST' && request.url().includes('/api/tmux/sessions')) {
        created.push(JSON.parse(request.postData() || '{}'))
      }
    })

    await page.goto('/')
    await page.waitForSelector('.dashboard')

    const launcher = page.locator('.terminal-window').first().locator('.launcher')
    await expect(launcher.getByLabel('Launch flags')).toHaveValue('--dangerously-skip-permissions')
    await expect(launcher.locator('.launcher-preview')).toHaveText('claude --dangerously-skip-permissions')

    await launcher.getByRole('button', { name: 'Flags…' }).click()
    const panel = launcher.locator('.flag-panel')
    await expect(panel).toBeVisible()

    await panel.getByLabel('Search flags').fill('continue')
    await expect(panel.locator('.flag-row')).toHaveCount(1)
    await panel.getByRole('button', { name: /--continue/ }).click()

    await expect(launcher.getByLabel('Launch flags')).toHaveValue('--dangerously-skip-permissions --continue')
    await expect(launcher.locator('.launcher-preview'))
      .toHaveText('claude --dangerously-skip-permissions --continue')

    await launcher.getByRole('button', { name: 'Launch claude in chrote' }).click()

    await expect.poll(() => created).toHaveLength(1)
    expect(created[0]).toMatchObject({
      harness: 'claude-code',
      flags: '--dangerously-skip-permissions --continue',
    })
  })

  test('the catalogue docks beside the launcher with room, and stacks under it without', async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    // One window to the workspace, so the launcher has a tile wide enough for
    // two columns to be a question the container query can answer either way.
    await page.keyboard.press('Alt+-')

    const launcher = page.locator('.terminal-window').first().locator('.launcher')
    await launcher.getByRole('button', { name: 'Flags…' }).click()
    const body = launcher.locator('.launcher-body')
    const panel = launcher.locator('.flag-panel')
    await expect(panel).toBeVisible()

    const rightOfBody = async () => {
      const [bodyBox, panelBox] = [await body.boundingBox(), await panel.boundingBox()]
      return bodyBox !== null && panelBox !== null && panelBox.x >= bodyBox.x + bodyBox.width
    }
    const belowBody = async () => {
      const [bodyBox, panelBox] = [await body.boundingBox(), await panel.boundingBox()]
      return bodyBox !== null && panelBox !== null && panelBox.y >= bodyBox.y + bodyBox.height
    }

    await expect.poll(rightOfBody).toBe(true)

    await page.setViewportSize({ width: 640, height: 900 })
    await expect.poll(belowBody).toBe(true)
    await expect.poll(rightOfBody).toBe(false)
  })
})
