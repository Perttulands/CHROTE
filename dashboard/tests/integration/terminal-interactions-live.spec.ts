import { expect, test, type Page } from '@playwright/test'

async function openLiveDashboard(page: Page) {
  const health = await page.request.get('/api/health')
  expect(health.ok()).toBe(true)

  await page.goto('/')
  await expect(page.locator('.dashboard')).toBeVisible()
  await expect(page.locator('.terminal-grid[data-workspace="terminal1"]')).toBeVisible()
}

test.describe('live terminal interactions', () => {
  test('desktop keeps terminal input native while exposing visible assignment controls', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await openLiveDashboard(page)

    const terminalOneTab = page.locator('.tab-bar .tab.active').first()
    await expect(terminalOneTab).toHaveClass(/active/)
    await terminalOneTab.focus()
    await page.keyboard.press('Tab')
    await expect(terminalOneTab).toHaveClass(/active/)
    await expect(terminalOneTab).not.toBeFocused()

    const workspace = page.locator('.terminal-workspace-dock[data-active="true"]')
    const sessionsSidecar = workspace.getByRole('button', { name: 'Sessions sidecar', exact: true })
    await expect(sessionsSidecar).toHaveAttribute('aria-expanded', 'false')
    await sessionsSidecar.click()
    await expect(sessionsSidecar).toHaveAttribute('aria-expanded', 'true')

    const sessionRow = workspace.locator('.session-item').first()
    await expect(sessionRow).toBeVisible()
    const sessionName = (await sessionRow.locator('.session-name').textContent())?.trim()
    expect(sessionName).toBeTruthy()

    const firstWindow = workspace.locator('.terminal-window:visible').first()
    await expect(sessionRow.getByRole('button', { name: `Session actions for ${sessionName}` })).toBeVisible()
    const rowBox = await sessionRow.boundingBox()
    const windowBox = await firstWindow.boundingBox()
    expect(rowBox).toBeTruthy()
    expect(windowBox).toBeTruthy()
    await page.mouse.move(rowBox!.x + rowBox!.width / 2, rowBox!.y + rowBox!.height / 2)
    await page.mouse.down()
    await page.mouse.move(rowBox!.x + rowBox!.width / 2 + 12, rowBox!.y + rowBox!.height / 2 + 12, { steps: 4 })
    await page.mouse.move(windowBox!.x + windowBox!.width / 2, windowBox!.y + windowBox!.height / 2, { steps: 10 })
    await page.mouse.up()

    await expect(firstWindow.locator('.tag-name')).toHaveText(sessionName!)
    await sessionsSidecar.click()
    await expect(sessionsSidecar).toHaveAttribute('aria-expanded', 'false')
    const terminalFrame = firstWindow.locator(`iframe[title="Terminal - ${sessionName}"]`)
    await expect(terminalFrame).toBeVisible({ timeout: 15000 })

    const frameBody = terminalFrame.contentFrame().locator('body')
    await expect(frameBody).toBeVisible({ timeout: 15000 })
    await frameBody.evaluate(() => {
      const marker = window as Window & { __chroteContextMenuPrevented?: boolean }
      marker.__chroteContextMenuPrevented = true
      document.addEventListener('contextmenu', event => {
        marker.__chroteContextMenuPrevented = event.defaultPrevented
      }, { once: true })
    })
    await frameBody.click({ button: 'right', position: { x: 20, y: 20 } })
    await expect.poll(() => frameBody.evaluate(() => (
      window as Window & { __chroteContextMenuPrevented?: boolean }
    ).__chroteContextMenuPrevented)).toBe(false)
  })

  test('mobile defaults to a terminal-first layout with reachable session actions', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openLiveDashboard(page)

    const workspace = page.locator('.terminal-workspace-dock[data-active="true"]')
    const sessionsSidecar = workspace.getByRole('button', { name: 'Sessions sidecar', exact: true })
    await expect(sessionsSidecar).toHaveAttribute('aria-expanded', 'false')
    await expect(workspace.locator('.terminal-window:visible')).toHaveCount(1)

    const layoutFour = page.getByRole('button', { name: '4', exact: true }).last()
    await expect(layoutFour).toBeVisible()
    const hitTargetIsLayoutButton = await layoutFour.evaluate(button => {
      const rect = button.getBoundingClientRect()
      const hit = document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2)
      return hit === button || button.contains(hit)
    })
    expect(hitTargetIsLayoutButton).toBe(true)
    await layoutFour.click()
    await expect(page.getByRole('group', { name: 'Window view controls' }).getByRole('button')).toHaveCount(4)

    await sessionsSidecar.click()
    await expect(sessionsSidecar).toHaveAttribute('aria-expanded', 'true')
    const panel = workspace.locator('.session-panel')
    await expect(panel).toBeVisible()
    const sessionRow = panel.locator('.session-item').first()
    await expect(sessionRow).toBeVisible()
    const actions = sessionRow.getByRole('button', { name: /Session actions for/ })
    await actions.click()
    await expect(page.locator('.session-context-menu')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.locator('.session-context-menu')).toHaveCount(0)
  })
})
