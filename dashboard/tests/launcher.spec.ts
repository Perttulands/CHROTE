import { test, expect } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { setWorkspaceState } from './helpers'

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

  test('switching harnesses moves neither the harness buttons nor the frame', async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    const launcher = page.locator('.terminal-window').first().locator('.launcher')
    const shell = launcher.getByRole('button', { name: 'Shell', exact: true })
    await expect(launcher.getByLabel('Launch flags')).toBeEnabled()
    const frameBefore = await launcher.boundingBox()
    const shellBefore = await shell.boundingBox()

    await shell.click()
    await expect(launcher.getByLabel('Launch flags')).toBeDisabled()
    expect(await launcher.boundingBox()).toEqual(frameBefore)
    expect(await shell.boundingBox()).toEqual(shellBefore)

    await launcher.getByRole('button', { name: 'Claude Code' }).click()
    await launcher.getByLabel('Launch flags').fill('--verbose')
    await expect(launcher.getByRole('button', { name: 'Reset' })).toBeEnabled()
    expect(await launcher.boundingBox()).toEqual(frameBefore)
    expect(await shell.boundingBox()).toEqual(shellBefore)
  })

  test('a fourth-window launcher reflows in short wide tiles and scrolls from its first control on a phone', async ({ page }) => {
    await mockApiRoutes(page)
    const windows = Array.from({ length: 4 }, (_, i) => ({
      id: `terminal1-window-${i}`, boundSessions: [], activeSession: null, colorIndex: i,
    }))
    await setWorkspaceState(page, {
      workspaces: {
        terminal1: { windowCount: 4, windows },
        terminal2: { windowCount: 1, windows: [] },
      },
    })
    await page.setViewportSize({ width: 1920, height: 1080 })
    await page.goto('/')
    const tile = page.locator('.terminal-window').nth(3)
    const launcher = tile.locator('.launcher')
    const scroller = tile.locator('.empty-window-state')
    const harness = launcher.getByRole('button', { name: 'Claude Code' })
    const folder = launcher.getByLabel('Folder', { exact: true })
    const launch = launcher.getByRole('button', { name: 'Launch claude in chrote' })

    // Four tiles leave plenty of width but little height: use both columns,
    // and make every step reachable even when the window gets shorter.
    for (const viewport of [{ width: 1920, height: 1080 }, { width: 1280, height: 800 }]) {
      await page.setViewportSize(viewport)
      const location = await launcher.locator('.launcher-location').boundingBox()
      const options = await launcher.locator('.launcher-options').boundingBox()
      expect(options!.x).toBeGreaterThan(location!.x + location!.width)
      await scroller.evaluate(element => { element.scrollTop = 0 })
      await expect(harness).toBeInViewport({ ratio: 1 })
      await folder.scrollIntoViewIfNeeded()
      await expect(folder).toBeInViewport({ ratio: 1 })
      await launch.scrollIntoViewIfNeeded()
      await expect(launch).toBeInViewport({ ratio: 1 })
    }

    await page.setViewportSize({ width: 390, height: 400 })
    await page.getByRole('button', { name: 'Close Sessions sidecar', exact: true }).click()
    await page.locator('.mobile-controls-row .layout-btn').nth(3).click()
    await scroller.evaluate(element => { element.scrollTop = 0 })
    await expect(harness).toBeInViewport({ ratio: 1 })
    await folder.fill('/srv/other')
    await launcher.getByRole('button', { name: 'Launch claude in other' }).click()
    await expect(tile.locator('.session-tag')).toHaveCount(1)
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
