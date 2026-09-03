import { expect, test, type Locator, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

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

test.describe('terminal workspace sidecars', () => {

  test('opens Sessions and focuses its filter when slash is pressed while closed', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })
    const sessionsTrigger = page.getByRole('button', { name: 'Sessions sidecar', exact: true })
    await expect(sessionsTrigger).toHaveAttribute('aria-expanded', 'false')

    await page.keyboard.press('/')

    await expect(sessionsTrigger).toHaveAttribute('aria-expanded', 'true')
    await expect(page.locator('.session-panel.sidecar-pinned')).toBeVisible()
    await expect(page.locator('.session-search-input')).toBeFocused()
  })

  test('keeps open desktop sidecars beside terminal content', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })

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
  })

  test('uses icon-only hit-testable launchers and independent overlay drawers on narrow screens', async ({ page }) => {
    await openFreshTerminal(page, { width: 700, height: 800 })

    const dock = page.locator('.terminal-workspace-dock[data-active="true"]')
    const terminal = dock.locator('.terminal-area')
    const initial = await box(terminal)
    const sessionsTrigger = page.getByRole('button', { name: 'Sessions sidecar', exact: true })
    const filesTrigger = page.getByRole('button', { name: 'Files sidecar' })

    await expect(sessionsTrigger.locator('.terminal-sidecar-label')).toHaveCSS('display', 'none')
    await expect(filesTrigger.locator('.terminal-sidecar-label')).toHaveCSS('display', 'none')
    await expect(dock.locator('.session-panel, .terminal-files-panel')).toHaveCount(0)

    await sessionsTrigger.click()
    const sessionsPanel = dock.locator('.session-panel.sidecar-overlay')
    await expect(sessionsPanel).toBeVisible()
    await expect(page.getByRole('button', { name: 'Pin Sessions sidecar' })).toHaveCount(0)
    const overlay = await box(sessionsPanel)
    expect(overlay.width).toBeLessThanOrEqual(380)
    const unchanged = await box(terminal)
    expect(Math.abs(unchanged.width - initial.width)).toBeLessThanOrEqual(1)

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

  // The T2 W1 chip is gone. The row says where the operator is by being marked
  // when the focused tile is showing its session, and by nothing otherwise.
  test('peeks an attached session from the row and marks the row the focused tile shows', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })
    await page.getByRole('button', { name: 'Sessions sidecar' }).click()

    const row = page.locator('.session-item').filter({ hasText: 'hq-mayor' })
    await expect(row).toBeVisible()
    await row.click({ button: 'right' })
    await page.getByRole('menuitem', { name: /Attach to window/ }).click()
    await page.locator('.menu-submenu .menu-row').filter({ hasText: 'Terminal 2 - Window 1' }).click()

    await expect(row.locator('.window-location-chip')).toHaveCount(0)
    await expect(row).not.toHaveClass(/in-focused-tile/)

    await row.locator('.session-name').click()
    await expect(page.locator('.sheet-left')).toBeVisible()
    await expect(page.locator('.tab.active')).toContainText(/^Terminal ?▾?$/)

    await page.keyboard.press('Escape')
    await expect(page.locator('.sheet-left')).toHaveCount(0)
    await expect(row).toBeVisible()

    // Go where the session went, focus its tile, and the row is marked.
    await page.keyboard.press('Alt+2')
    await expect(page.locator('.tab.active')).toContainText('Terminal 2')
    const tile = page.locator('.terminal-workspace-dock[data-workspace="terminal2"] .terminal-window').first()
    await expect(tile.locator('.session-tag')).toContainText('hq-mayor')
    await tile.locator('.terminal-window-body').click()
    await expect(tile).toHaveClass(/focused/)
    await expect(row).toHaveClass(/in-focused-tile/)
  })
})
