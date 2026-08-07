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
  await page.route(/\/terminal(\/|\?|$)/, route => route.fulfill({
    status: 200,
    contentType: 'text/html',
    body: '<html><body>mock terminal</body></html>',
  }))
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Sessions sidecar' })).toBeVisible()
}

test.describe('terminal workspace sidecars', () => {
  test('keeps an untouched fresh sidecar closed across reload', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })
    const dock = page.locator('.terminal-workspace-dock[data-active="true"]')
    const sessionsTrigger = page.getByRole('button', { name: 'Sessions sidecar', exact: true })

    await expect(dock.locator('.session-panel, .terminal-files-panel')).toHaveCount(0)
    await expect(sessionsTrigger).toHaveAttribute('aria-pressed', 'false')
    await page.reload()

    await expect(sessionsTrigger).toBeVisible()
    await expect(sessionsTrigger).toHaveAttribute('aria-pressed', 'false')
    await expect(dock.locator('.session-panel, .terminal-files-panel')).toHaveCount(0)
    await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('chrote.sessionsDock.v1') || '{}')
      ?.state)).toEqual({ open: false, pinned: false, width: 260 })
  })

  test('opens Sessions and focuses its filter when slash is pressed while closed', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })
    const sessionsTrigger = page.getByRole('button', { name: 'Sessions sidecar', exact: true })
    await expect(sessionsTrigger).toHaveAttribute('aria-expanded', 'false')

    await page.keyboard.press('/')

    await expect(sessionsTrigger).toHaveAttribute('aria-expanded', 'true')
    await expect(page.locator('.session-panel.sidecar-overlay')).toBeVisible()
    await expect(page.locator('.session-search-input')).toBeFocused()
  })

  test('keeps closed and overlay sidecars out of terminal layout width, then pins explicitly', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })

    const dock = page.locator('.terminal-workspace-dock[data-active="true"]')
    const terminal = dock.locator('.terminal-area')
    const initial = await box(terminal)
    await expect(dock.locator('.session-panel, .terminal-files-panel')).toHaveCount(0)

    await page.getByRole('button', { name: 'Sessions sidecar' }).click()
    await expect(dock.locator('.session-panel.sidecar-overlay')).toBeVisible()
    await expect(dock.locator('.terminal-sidecar-dismiss')).toBeVisible()
    const overlaid = await box(terminal)
    expect(Math.abs(overlaid.x - initial.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(overlaid.width - initial.width)).toBeLessThanOrEqual(1)

    await page.getByRole('button', { name: 'Files sidecar' }).click()
    await expect(dock.locator('.session-panel.sidecar-pinned')).toBeVisible()
    await expect(dock.locator('.terminal-files-panel.sidecar-pinned')).toBeVisible()
    await expect(dock.locator('.terminal-sidecar-dismiss')).toHaveCount(0)

    await page.getByRole('button', { name: 'Close Sessions sidecar' }).click()
    await expect(dock.locator('.session-panel')).toHaveCount(0)
    await expect(dock.locator('.terminal-files-panel.sidecar-overlay')).toBeVisible()

    await page.getByRole('button', { name: 'Pin Files sidecar' }).click()
    await expect(dock.locator('.terminal-files-panel.sidecar-pinned')).toBeVisible()
    await expect(dock.locator('.terminal-sidecar-dismiss')).toHaveCount(0)
    const pinned = await box(terminal)
    expect(pinned.width).toBeLessThan(initial.width - 200)

    await page.getByRole('button', { name: 'Close Files sidecar' }).click()
    await expect(dock.locator('.terminal-files-panel')).toHaveCount(0)
    const restored = await box(terminal)
    expect(Math.abs(restored.x - initial.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(restored.width - initial.width)).toBeLessThanOrEqual(1)

    // The pin preference survives close: reopening pins the panel beside the
    // terminal again instead of overlaying it.
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

  test('Escape ignores hidden keep-alive dialogs and defers to visible dialogs', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })
    await page.evaluate(() => {
      const host = document.createElement('div')
      host.id = 'escape-blocker-host'
      host.style.display = 'none'
      const dialog = document.createElement('section')
      dialog.setAttribute('role', 'dialog')
      dialog.style.width = '120px'
      dialog.style.height = '80px'
      host.append(dialog)
      document.body.append(host)
    })

    await page.getByRole('button', { name: 'Sessions sidecar' }).click()
    await page.keyboard.press('Escape')
    await expect(page.locator('.session-panel')).toHaveCount(0)

    await page.getByRole('button', { name: 'Sessions sidecar' }).click()
    await page.locator('#escape-blocker-host').evaluate(element => {
      ;(element as HTMLElement).style.display = 'block'
    })
    await page.keyboard.press('Escape')
    await expect(page.locator('.session-panel.sidecar-overlay')).toBeVisible()

    await page.locator('#escape-blocker-host').evaluate(element => element.remove())
    await page.keyboard.press('Escape')
    await expect(page.locator('.session-panel')).toHaveCount(0)
  })

  test('peeks an attached session from the row and reserves location-chip click for frame navigation', async ({ page }) => {
    await openFreshTerminal(page, { width: 1280, height: 800 })
    await page.getByRole('button', { name: 'Sessions sidecar' }).click()

    const row = page.locator('.session-item').filter({ hasText: 'hq-mayor' })
    await expect(row).toBeVisible()
    await row.click({ button: 'right' })
    await page.getByRole('button', { name: /Attach to Window/ }).click()
    await page.locator('.session-context-submenu .session-context-item').filter({ hasText: 'Terminal 2 - Window 1' }).click()

    const location = row.getByRole('button', { name: 'Focus assigned window T2 W1' })
    await expect(location).toBeVisible()
    await row.locator('.session-name').click()
    await expect(page.locator('.floating-modal')).toBeVisible()
    await expect(page.locator('.tab.active')).toContainText(/^Terminal$/)
    await expect(location).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(page.locator('.floating-modal')).toHaveCount(0)
    await expect(row).toBeVisible()

    await location.click()
    await expect(page.locator('.tab.active')).toContainText('Terminal 2')
    await expect(page.locator('.terminal-window:visible').first().locator('.session-tag')).toContainText('hq-mayor')
  })
})
