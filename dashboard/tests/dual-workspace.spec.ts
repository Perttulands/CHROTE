import { test, expect, Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { openSessionsSidecar } from './helpers'

/**
 * Dual workspace (Terminal 1 / Terminal 2) interaction tests.
 *
 * Both terminal areas are always rendered in the DOM. The active one uses
 * display:contents while the hidden one uses display:none (App.tsx).
 * The terminal-grid elements carry data-workspace attributes.
 *
 * Bead: pol-ddf8
 */

// Helper: drag-and-drop for dnd-kit (requires minimum distance to activate)
async function dragAndDrop(page: Page, sourceSelector: string, targetSelector: string) {
  const source = page.locator(sourceSelector).first()
  const target = page.locator(targetSelector).first()

  const sourceBox = await source.boundingBox()
  const targetBox = await target.boundingBox()

  if (!sourceBox || !targetBox) {
    throw new Error('Could not find source or target element')
  }

  const startX = sourceBox.x + sourceBox.width / 2
  const startY = sourceBox.y + sourceBox.height / 2
  const endX = targetBox.x + targetBox.width / 2
  const endY = targetBox.y + targetBox.height / 2

  await page.mouse.move(startX, startY)
  await page.mouse.down()
  await page.mouse.move(startX + 10, startY + 10, { steps: 5 })
  await page.mouse.move(endX, endY, { steps: 10 })
  // drag settle — no event to wait for
  await page.waitForTimeout(100) // drag settle — no event to wait for
  await page.mouse.up()
  // drag settle — no event to wait for
  await page.waitForTimeout(100) // drag settle — no event to wait for
}

// Scoped locators for each workspace
function terminal1Grid(page: Page) {
  return page.locator('.terminal-grid[data-workspace="terminal1"]')
}
function terminal2Grid(page: Page) {
  return page.locator('.terminal-grid[data-workspace="terminal2"]')
}

test.describe('Dual Workspace: Terminal 1 & Terminal 2', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    // Clear persisted dashboard state before first app boot without affecting
    // reloads later in the same test.
    await page.addInitScript(() => {
      const clearFlag = '__chrote_dual_workspace_storage_cleared'
      if (sessionStorage.getItem(clearFlag) === '1') return
      localStorage.clear()
      sessionStorage.setItem(clearFlag, '1')
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page)
    await page.getByRole('button', { name: 'Terminal 2', exact: true }).click()
    await openSessionsSidecar(page)
    await page.getByRole('button', { name: 'Terminal', exact: true }).click()
    await page.waitForSelector('.session-item')
  })

  test.describe('Tab Switching Visibility', () => {

    test('clicking Terminal 2 tab shows terminal2 and hides terminal1', async ({ page }) => {
      await page.click('.tab:has-text("Terminal 2")')

      // terminal2 should now be visible
      await expect(terminal2Grid(page)).toBeVisible()

      // terminal1 should be hidden
      await expect(terminal1Grid(page)).not.toBeVisible()
    })

  })

  test.describe('Workspace Isolation', () => {

    test('binding same session cross-workspace moves it (dedup)', async ({ page }) => {
      // Bind jack to terminal1
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("jack")',
        '[data-workspace="terminal1"] .terminal-window',
      )
      const t1Window = terminal1Grid(page).locator('.terminal-window').first()
      await expect(t1Window.locator('.tag-name')).toContainText('jack')

      // Move the already-assigned session to Terminal 2 using the same
      // assignment path exposed in the session context menu. This covers the
      // dedup behavior without depending on dragging an assigned sidebar item.
      const sessionItem = page.locator('.session-panel .session-item:has-text("jack")')
      await sessionItem.click({ button: 'right' })
      const menu = page.locator('.session-context-menu')
      await expect(menu).toBeVisible()
      await menu.getByRole('button', { name: /Attach to Window/ }).click()
      await page.locator('.session-context-submenu .session-context-item:has-text("Terminal 2 - Window 1")').click()

      // jack should now be in terminal2
      await page.click('.tab:has-text("Terminal 2")')
      await expect(terminal2Grid(page)).toBeVisible()
      const t2Window = terminal2Grid(page).locator('.terminal-window').first()
      await expect(t2Window.locator('.tag-name')).toContainText('jack', { timeout: 10000 })

      // Switch back to Terminal 1 - jack should be gone
      await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()
      await expect(t1Window.locator('.session-tag')).toHaveCount(0)
    })
  })

  test.describe('Workspace State Preservation on Tab Switch', () => {
    test('switching between Terminal 1 and 2 preserves each workspace state', async ({ page }) => {
      // Bind jack to terminal1 window 0
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("jack")',
        '[data-workspace="terminal1"] .terminal-window >> nth=0',
      )

      // Bind mayor to terminal1 window 1
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("mayor")',
        '[data-workspace="terminal1"] .terminal-window >> nth=1',
      )

      // Switch to Terminal 2
      await page.click('.tab:has-text("Terminal 2")')

      // Bind lizzy to terminal2 window 0
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("lizzy")',
        '[data-workspace="terminal2"] .terminal-window >> nth=0',
      )

      // Bind darcy to terminal2 window 1
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("darcy")',
        '[data-workspace="terminal2"] .terminal-window >> nth=1',
      )

      // Verify terminal2 state
      const t2Windows = terminal2Grid(page).locator('.terminal-window')
      await expect(t2Windows.nth(0).locator('.tag-name')).toContainText('lizzy')
      await expect(t2Windows.nth(1).locator('.tag-name')).toContainText('darcy')

      // Switch back to Terminal 1
      await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()

      // terminal1 state should be intact
      const t1Windows = terminal1Grid(page).locator('.terminal-window')
      await expect(t1Windows.nth(0).locator('.tag-name')).toContainText('jack')
      await expect(t1Windows.nth(1).locator('.tag-name')).toContainText('mayor')

      // Switch to Terminal 2 again
      await page.click('.tab:has-text("Terminal 2")')

      // terminal2 state should still be intact
      await expect(t2Windows.nth(0).locator('.tag-name')).toContainText('lizzy')
      await expect(t2Windows.nth(1).locator('.tag-name')).toContainText('darcy')
    })

  })

  test.describe('Persistence Across Reload', () => {
    test('both workspaces persist independently after reload', async ({ page }) => {
      // Bind jack to terminal1 window 0
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("jack")',
        '[data-workspace="terminal1"] .terminal-window >> nth=0',
      )

      // Switch to Terminal 2 and bind lizzy
      await page.click('.tab:has-text("Terminal 2")')
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("lizzy")',
        '[data-workspace="terminal2"] .terminal-window >> nth=0',
      )

      // Reload the page (mocks were registered in beforeEach and survive navigation)
      await page.reload()
      await page.waitForSelector('.dashboard')
      await page.waitForSelector('.session-item')

      // Terminal 1 should still have jack
      // After reload, default tab is terminal1
      const t1Window = terminal1Grid(page).locator('.terminal-window').first()
      await expect(t1Window.locator('.tag-name')).toContainText('jack')

      // Switch to Terminal 2 - should still have lizzy
      await page.click('.tab:has-text("Terminal 2")')
      const t2Window = terminal2Grid(page).locator('.terminal-window').first()
      await expect(t2Window.locator('.tag-name')).toContainText('lizzy')
    })

  })

})
