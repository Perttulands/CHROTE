import { expect, test, type Locator, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { dragAndDrop, openSessionsSidecar } from './helpers'

async function box(locator: Locator) {
  const value = await locator.boundingBox()
  if (!value) throw new Error('expected a rendered bounding box')
  return value
}

async function openFreshTerminal(page: Page, viewport?: { width: number; height: number }) {
  if (viewport) await page.setViewportSize(viewport)
  await mockApiRoutes(page)
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Sessions sidecar' })).toBeVisible()
}

async function openWithSessions(page: Page) {
  await mockApiRoutes(page)
  await page.goto('/')
  await page.waitForSelector('.dashboard')
  await openSessionsSidecar(page)
}

test.describe('terminal workspace sidecars', () => {

  // One page carries the whole sidecar geometry contract, because mounting the
  // dashboard costs more than any assertion in it: the chord that opens Files,
  // the desktop rail that takes its room from the terminal, and the same
  // triggers on a narrow screen, where the panels overlay instead.
  test('open on their chord, sit beside terminal content on a desktop, and overlay it on a narrow screen', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })

    const filesTrigger = page.getByRole('button', { name: 'Files sidecar', exact: true })
    await expect(filesTrigger).toHaveAttribute('aria-expanded', 'false')

    await page.keyboard.press('Alt+o')

    await expect(filesTrigger).toHaveAttribute('aria-expanded', 'true')
    await expect(page.getByRole('textbox', { name: 'Find files' })).toBeFocused()

    await page.keyboard.press('Alt+o')
    await expect(filesTrigger).toHaveAttribute('aria-expanded', 'false')

    const dock = page.locator('.terminal-workspace-dock[data-active="true"]')
    const terminal = dock.locator('.terminal-area')
    const initial = await box(terminal)
    await expect(dock.locator('.session-panel, .terminal-files-panel')).toHaveCount(0)

    await page.getByRole('button', { name: 'Sessions sidecar' }).click()
    const sessionsPanel = dock.locator('.session-panel.sidecar-pinned')
    await expect(sessionsPanel).toBeVisible()
    await expect(page.getByRole('button', { name: 'Pin Sessions sidecar' })).toHaveCount(0)
    await expect(dock.locator('.terminal-sidecar-dismiss')).toHaveCount(0)
    const sessionsBox = await box(sessionsPanel)
    const besideSessions = await box(terminal)
    expect(sessionsBox.x + sessionsBox.width).toBeLessThanOrEqual(besideSessions.x + 1)
    expect(besideSessions.width).toBeLessThan(initial.width - 200)

    await page.getByRole('button', { name: 'Files sidecar' }).click()
    await expect(dock.locator('.session-panel.sidecar-pinned')).toBeVisible()
    await expect(dock.locator('.terminal-files-panel.sidecar-pinned')).toBeVisible()
    await expect(dock.locator('.terminal-sidecar-dismiss')).toHaveCount(0)

    await page.getByRole('button', { name: 'Close Sessions sidecar' }).click()
    await expect(dock.locator('.session-panel')).toHaveCount(0)
    await expect(dock.locator('.terminal-files-panel.sidecar-pinned')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Pin Files sidecar' })).toHaveCount(0)
    await expect(dock.locator('.terminal-sidecar-dismiss')).toHaveCount(0)
    const pinned = await box(terminal)
    expect(pinned.width).toBeLessThan(initial.width - 200)

    await page.getByRole('button', { name: 'Close Files sidecar' }).click()
    await expect(dock.locator('.terminal-files-panel')).toHaveCount(0)
    const restored = await box(terminal)
    expect(Math.abs(restored.x - initial.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(restored.width - initial.width)).toBeLessThanOrEqual(1)

    // Reopening keeps the desktop panel beside the terminal without changing
    // the stored preference used by narrow viewports.
    await page.getByRole('button', { name: 'Files sidecar' }).click()
    await expect(dock.locator('.terminal-files-panel.sidecar-pinned')).toBeVisible()
    await expect(dock.locator('.terminal-sidecar-dismiss')).toHaveCount(0)
    const reopened = await box(terminal)
    expect(reopened.width).toBeLessThan(initial.width - 200)

    await page.getByRole('button', { name: 'Close Files sidecar' }).click()
    await expect(dock.locator('.terminal-files-panel')).toHaveCount(0)

    // The same triggers on a narrow screen: icon-only, and the panels float
    // over the terminal rather than taking a column it cannot spare.
    await page.setViewportSize({ width: 700, height: 800 })
    const narrowInitial = await box(terminal)
    const sessionsTrigger = page.getByRole('button', { name: 'Sessions sidecar', exact: true })

    await expect(sessionsTrigger.locator('.terminal-sidecar-label')).toHaveCSS('display', 'none')
    await expect(filesTrigger.locator('.terminal-sidecar-label')).toHaveCSS('display', 'none')

    await sessionsTrigger.click()
    const overlayPanel = dock.locator('.session-panel.sidecar-overlay')
    await expect(overlayPanel).toBeVisible()
    await expect(page.getByRole('button', { name: 'Pin Sessions sidecar' })).toHaveCount(0)
    const overlay = await box(overlayPanel)
    expect(overlay.width).toBeLessThanOrEqual(380)
    const unchanged = await box(terminal)
    expect(Math.abs(unchanged.width - narrowInitial.width)).toBeLessThanOrEqual(1)

    // The open drawer may not swallow the trigger for the other one.
    const filesCenter = await filesTrigger.evaluate(element => {
      const rect = element.getBoundingClientRect()
      const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2)
      return hit?.closest('button')?.getAttribute('aria-label')
    })
    expect(filesCenter).toBe('Files sidecar')

    await filesTrigger.click()
    await expect(dock.locator('.session-panel.sidecar-overlay')).toBeVisible()
    await expect(dock.locator('.terminal-files-panel.sidecar-overlay')).toBeVisible()
    await dock.locator('.terminal-sidecar-dismiss').click({ position: { x: 650, y: 300 } })
    await expect(dock.locator('.session-panel')).toHaveCount(0)
    await expect(dock.locator('.terminal-files-panel')).toHaveCount(0)
  })

  test('shares Sessions across terminal tabs while Files follows its terminal workspace', async ({ page }) => {
    await openWithSessions(page)
    await page.route('**/api/scheduled-tasks', route => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, data: { tasks: [] } }),
    }))
    const sessions = page.locator('.session-panel')
    const files = page.locator('.terminal-files-panel')

    await expect(sessions).toHaveClass(/sidecar-pinned/)
    await expect(page.getByRole('button', { name: 'Unpin Sessions sidecar' })).toHaveCount(0)
    await page.getByRole('button', { name: 'Files sidecar', exact: true }).click()
    await expect(files).toBeVisible()
    await expect(sessions).toHaveClass(/sidecar-pinned/)
    const sessionsWidth = await sessions.evaluate(element => element.getBoundingClientRect().width)

    const filter = page.getByPlaceholder('Filter sessions...')
    await filter.fill('hq')
    const groupHeader = page.locator('.session-group-header').first()
    const groupName = await groupHeader.locator('.group-name').innerText()
    await groupHeader.click()
    await expect(groupHeader.locator('.expand-icon')).toHaveText('▶')

    await page.locator('.tab-bar-tabs .tab').filter({ hasText: /^Terminal 2$/ }).click()

    await expect(page.getByRole('button', { name: 'Sessions sidecar', exact: true })).toHaveAttribute('aria-pressed', 'true')
    await expect(sessions).toBeVisible()
    await expect(sessions).toHaveClass(/sidecar-pinned/)
    await expect.poll(() => sessions.evaluate(element => element.getBoundingClientRect().width)).toBe(sessionsWidth)
    await expect(filter).toHaveValue('hq')
    await expect(page.locator('.session-group').filter({ hasText: groupName }).locator('.expand-icon')).toHaveText('▶')
    await expect(page.getByRole('button', { name: 'Files sidecar', exact: true })).toHaveAttribute('aria-pressed', 'false')
    await expect(files).toHaveCount(0)

    await page.locator('.tab-bar-tabs .tab').filter({ hasText: /^Scheduled$/ }).click()

    await expect(sessions).toBeVisible()
    await expect(sessions).toHaveAttribute('data-active-workspace', 'terminal2')
    await expect(sessions).toHaveClass(/sidecar-pinned/)
    await expect.poll(() => sessions.evaluate(element => element.getBoundingClientRect().width)).toBe(sessionsWidth)
    await expect(filter).toHaveValue('hq')
    await expect(page.locator('.session-group').filter({ hasText: groupName }).locator('.expand-icon')).toHaveText('▶')
    await page.getByRole('button', { name: 'Close Sessions sidecar' }).click()
    await expect(sessions).toHaveCount(0)

    await page.locator('.tab-bar-tabs .tab').filter({ hasText: /^Terminal 2$/ }).click()
    await expect(page.getByRole('button', { name: 'Sessions sidecar', exact: true })).toHaveAttribute('aria-pressed', 'false')

    await page.locator('.tab-bar-tabs .tab').filter({ hasText: /^Terminal$/ }).click()

    await expect(files).toBeVisible()
    await page.getByRole('button', { name: 'Sessions sidecar', exact: true }).click()
    await expect(sessions).toBeVisible()
    await expect(filter).toHaveValue('hq')
    await expect(page.locator('.session-group').filter({ hasText: groupName }).locator('.expand-icon')).toHaveText('▶')
  })

  test('keeps Sessions open while Files opens the file in the panel', async ({ page }) => {
    await openWithSessions(page)
    await page.route(/.*\/api\/files\/resources(?:\/.*)?$/, async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          isDir: true,
          items: [{ name: 'README.md', path: '/README.md', isDir: false, size: 12, modified: '2026-07-13T00:00:00Z', type: 'text/markdown' }],
        }),
      })
    })
    await page.route('**/api/files/raw/**', route => route.fulfill({
      status: 200,
      contentType: 'text/plain',
      body: '# Read me\n',
    }))
    await page.route('**/api/files/diff*', route => route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ path: '/README.md', repository: '', diff: '', truncated: false }),
    }))
    await page.reload()
    await openSessionsSidecar(page)
    await expect(page.locator('.session-panel')).toHaveClass(/sidecar-pinned/)
    await expect(page.locator('.terminal-files-panel')).toHaveCount(0)

    await dragAndDrop(page, '.session-item:has-text("hq-mayor")', '.terminal-window')
    const terminal = page.locator('.terminal-window-body .terminal-surface').first()
    await expect(terminal).toBeAttached()
    await terminal.evaluate(element => { element.setAttribute('data-dock-identity', 'preserved') })

    await page.getByRole('button', { name: 'Files sidecar', exact: true }).click()
    const files = page.locator('.terminal-files-panel')
    await expect(page.locator('.session-panel')).toHaveClass(/sidecar-pinned/)
    await expect(files).toHaveClass(/sidecar-pinned/)

    await page.getByRole('treeitem', { name: /File README\.md/ }).click()
    // The viewer replaces the tree inside the panel: nothing floats over the
    // terminals, and Back returns to where the operator was.
    await expect(files.getByRole('heading', { name: 'Read me' })).toBeVisible()
    await expect(files.getByRole('tree', { name: 'File tree' })).toHaveCount(0)
    await expect(page.locator('.file-peek')).toHaveCount(0)
    await files.getByRole('button', { name: 'Back' }).click()
    await expect(files.getByRole('tree', { name: 'File tree' })).toBeVisible()

    await page.getByRole('button', { name: 'Files sidecar', exact: true }).click()
    await expect(files).toHaveCount(0)
    await expect(page.locator('.session-panel')).toHaveClass(/sidecar-pinned/)
    await page.getByRole('button', { name: 'Sessions sidecar', exact: true }).click()
    await expect(page.locator('.session-panel')).toHaveCount(0)
    await expect(terminal).toHaveAttribute('data-dock-identity', 'preserved')
  })
})
