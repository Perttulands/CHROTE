import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'

// The launcher is the only way CHROTE starts a session, and in an empty window
// it is the window.
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

  test('the Folder field finds a workspace by a fragment and launches in a typed path', async ({ page }) => {
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
    const field = launcher.getByLabel('Folder', { exact: true })
    const before = await launcher.boundingBox()

    await field.fill('VSK')
    await expect(launcher.getByRole('option', { name: '/home/operator/repos/VSK-Zone' })).toHaveAttribute('aria-selected', 'true')

    await field.fill('/srv/other')
    await expect(launcher.getByLabel('Session name')).toHaveValue('claude-other')
    // Suggestions came and went; the launcher is exactly where and how big it was.
    expect(await launcher.boundingBox()).toEqual(before)

    await field.press('Enter')

    await expect.poll(() => created).toHaveLength(1)
    expect(created[0]).toMatchObject({ name: 'claude-other', cwd: '/srv/other', harness: 'claude-code' })
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
