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
    test('Terminal 1 is active and visible by default, Terminal 2 is hidden', async ({ page }) => {
      // terminal1 grid should be visible (parent has display:contents)
      await expect(terminal1Grid(page)).toBeVisible()

      // terminal2 grid's parent wrapper has display:none
      await expect(terminal2Grid(page)).not.toBeVisible()
    })

    test('clicking Terminal 2 tab shows terminal2 and hides terminal1', async ({ page }) => {
      await page.click('.tab:has-text("Terminal 2")')

      // terminal2 should now be visible
      await expect(terminal2Grid(page)).toBeVisible()

      // terminal1 should be hidden
      await expect(terminal1Grid(page)).not.toBeVisible()
    })

    test('clicking Terminal tab after Terminal 2 restores terminal1', async ({ page }) => {
      // Switch to Terminal 2
      await page.click('.tab:has-text("Terminal 2")')
      await expect(terminal2Grid(page)).toBeVisible()

      // Switch back to Terminal (1)
      // "Terminal" is the first tab; avoid matching "Terminal 2" by using exact click
      await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()
      await expect(terminal1Grid(page)).toBeVisible()
      await expect(terminal2Grid(page)).not.toBeVisible()
    })

    test('session panel is visible on both terminal tabs', async ({ page }) => {
      // Panel visible on terminal1
      await expect(page.locator('.session-panel')).toBeVisible()

      // Switch to terminal2
      await page.click('.tab:has-text("Terminal 2")')
      await expect(page.locator('.session-panel')).toBeVisible()
    })
  })

  test.describe('Workspace Isolation', () => {
    test('session bound to terminal1 does not appear in terminal2 windows', async ({ page }) => {
      // Bind jack to terminal1 window 0
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("jack")',
        '[data-workspace="terminal1"] .terminal-window',
      )

      // Verify jack tag in terminal1
      const t1Window = terminal1Grid(page).locator('.terminal-window').first()
      await expect(t1Window.locator('.tag-name')).toContainText('jack')

      // Switch to Terminal 2
      await page.click('.tab:has-text("Terminal 2")')

      // terminal2 windows should have no session tags
      const t2Windows = terminal2Grid(page).locator('.terminal-window')
      const t2Count = await t2Windows.count()
      for (let i = 0; i < t2Count; i++) {
        await expect(t2Windows.nth(i).locator('.session-tag')).toHaveCount(0)
      }
    })

    test('session bound to terminal2 does not appear in terminal1 windows', async ({ page }) => {
      // Switch to Terminal 2 first
      await page.click('.tab:has-text("Terminal 2")')

      // Bind lizzy to terminal2 window 0
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("lizzy")',
        '[data-workspace="terminal2"] .terminal-window',
      )

      // Verify lizzy in terminal2
      const t2Window = terminal2Grid(page).locator('.terminal-window').first()
      await expect(t2Window.locator('.tag-name')).toContainText('lizzy')

      // Switch back to Terminal 1
      await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()

      // terminal1 windows should have no session tags
      const t1Windows = terminal1Grid(page).locator('.terminal-window')
      const t1Count = await t1Windows.count()
      for (let i = 0; i < t1Count; i++) {
        await expect(t1Windows.nth(i).locator('.session-tag')).toHaveCount(0)
      }
    })

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

  test.describe('Drag to Terminal 2', () => {
    test('drag session from panel to terminal2 window works', async ({ page }) => {
      // Switch to Terminal 2
      await page.click('.tab:has-text("Terminal 2")')

      // Drag joe to terminal2 window
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("joe")',
        '[data-workspace="terminal2"] .terminal-window',
      )

      // Verify tag appears
      const t2Window = terminal2Grid(page).locator('.terminal-window').first()
      await expect(t2Window.locator('.session-tag')).toHaveCount(1)
      await expect(t2Window.locator('.tag-name')).toContainText('joe')
    })

    test('drag multiple sessions to different terminal2 windows', async ({ page }) => {
      // Switch to Terminal 2 (default 2 windows)
      await page.click('.tab:has-text("Terminal 2")')

      // Drag joe to first window
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("joe")',
        '[data-workspace="terminal2"] .terminal-window >> nth=0',
      )

      // Drag lizzy to second window
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("lizzy")',
        '[data-workspace="terminal2"] .terminal-window >> nth=1',
      )

      const t2Windows = terminal2Grid(page).locator('.terminal-window')
      await expect(t2Windows.nth(0).locator('.tag-name')).toContainText('joe')
      await expect(t2Windows.nth(1).locator('.tag-name')).toContainText('lizzy')
    })

    test('session shows as assigned in panel after drop to terminal2', async ({ page }) => {
      await page.click('.tab:has-text("Terminal 2")')

      const sessionItem = page.locator('.session-panel .session-item:has-text("joe")')
      await expect(sessionItem).not.toHaveClass(/assigned/)

      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("joe")',
        '[data-workspace="terminal2"] .terminal-window',
      )

      await expect(sessionItem).toHaveClass(/assigned/)
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

    test('window count changes in one workspace do not affect the other', async ({ page }) => {
      // Terminal 1 default is 2 windows
      await expect(terminal1Grid(page).locator('.terminal-window')).toHaveCount(2)

      // Switch terminal1 to 4 windows
      await page.locator('.terminal-area-controls:visible .layout-btn:has-text("4")').click()
      await expect(terminal1Grid(page).locator('.terminal-window')).toHaveCount(4)

      // Switch to Terminal 2
      await page.click('.tab:has-text("Terminal 2")')

      // terminal2 should still have default 2 windows
      await expect(terminal2Grid(page).locator('.terminal-window')).toHaveCount(2)

      // Switch terminal2 to 1 window
      await page.locator('.terminal-area-controls:visible .layout-btn:has-text("1")').click()
      await expect(terminal2Grid(page).locator('.terminal-window')).toHaveCount(1)

      // Switch back to Terminal 1 - should still be 4
      await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()
      await expect(terminal1Grid(page).locator('.terminal-window')).toHaveCount(4)
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

    test('window counts persist per workspace after reload', async ({ page }) => {
      // Set terminal1 to 4 windows
      await page.locator('.terminal-area-controls:visible .layout-btn:has-text("4")').click()
      await expect(terminal1Grid(page).locator('.terminal-window')).toHaveCount(4)

      // Set terminal2 to 1 window
      await page.click('.tab:has-text("Terminal 2")')
      await page.locator('.terminal-area-controls:visible .layout-btn:has-text("1")').click()
      await expect(terminal2Grid(page).locator('.terminal-window')).toHaveCount(1)

      // Reload
      await page.reload()
      await page.waitForSelector('.dashboard')

      // terminal1 should still be 4
      await expect(terminal1Grid(page).locator('.terminal-window')).toHaveCount(4)

      // terminal2 should still be 1
      await page.click('.tab:has-text("Terminal 2")')
      await expect(terminal2Grid(page).locator('.terminal-window')).toHaveCount(1)
    })

    test('multiple sessions per window persist in both workspaces after reload', async ({ page }) => {
      // Bind jack and joe to terminal1 window 0
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("jack")',
        '[data-workspace="terminal1"] .terminal-window >> nth=0',
      )
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("joe")',
        '[data-workspace="terminal1"] .terminal-window >> nth=0',
      )

      // Bind lizzy and darcy to terminal2 window 0
      await page.click('.tab:has-text("Terminal 2")')
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("lizzy")',
        '[data-workspace="terminal2"] .terminal-window >> nth=0',
      )
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("darcy")',
        '[data-workspace="terminal2"] .terminal-window >> nth=0',
      )

      // Verify terminal2 has both
      const t2Window = terminal2Grid(page).locator('.terminal-window').first()
      await expect(t2Window.locator('.session-tag')).toHaveCount(2)

      // Reload
      await page.reload()
      await page.waitForSelector('.dashboard')
      await page.waitForSelector('.session-item')

      // terminal1 should have both jack and joe
      const t1Window = terminal1Grid(page).locator('.terminal-window').first()
      await expect(t1Window.locator('.session-tag')).toHaveCount(2)
      await expect(t1Window.locator('.tag-name').nth(0)).toContainText('jack')
      await expect(t1Window.locator('.tag-name').nth(1)).toContainText('joe')

      // terminal2 should have both lizzy and darcy
      await page.click('.tab:has-text("Terminal 2")')
      const t2WindowAfter = terminal2Grid(page).locator('.terminal-window').first()
      await expect(t2WindowAfter.locator('.session-tag')).toHaveCount(2)
      await expect(t2WindowAfter.locator('.tag-name').nth(0)).toContainText('lizzy')
      await expect(t2WindowAfter.locator('.tag-name').nth(1)).toContainText('darcy')
    })
  })

  test.describe('Edge Cases', () => {
    test('switching to Files and back preserves both workspace states', async ({ page }) => {
      // Bind jack to terminal1
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("jack")',
        '[data-workspace="terminal1"] .terminal-window',
      )

      // Switch to Terminal 2, bind lizzy
      await page.click('.tab:has-text("Terminal 2")')
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("lizzy")',
        '[data-workspace="terminal2"] .terminal-window',
      )

      // Switch to Files
      await page.click('.tab:has-text("Files")')
      await expect(page.locator('.files-view')).toBeVisible()

      // Both terminal grids should be hidden
      await expect(terminal1Grid(page)).not.toBeVisible()
      await expect(terminal2Grid(page)).not.toBeVisible()

      // Switch back to Terminal 1
      await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()
      const t1Window = terminal1Grid(page).locator('.terminal-window').first()
      await expect(t1Window.locator('.tag-name')).toContainText('jack')

      // Switch to Terminal 2
      await page.click('.tab:has-text("Terminal 2")')
      const t2Window = terminal2Grid(page).locator('.terminal-window').first()
      await expect(t2Window.locator('.tag-name')).toContainText('lizzy')
    })

    test('removing session tag in terminal2 does not affect terminal1', async ({ page }) => {
      // Bind jack to terminal1
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("jack")',
        '[data-workspace="terminal1"] .terminal-window',
      )

      // Bind lizzy to terminal2
      await page.click('.tab:has-text("Terminal 2")')
      await dragAndDrop(
        page,
        '.session-panel .session-item:has-text("lizzy")',
        '[data-workspace="terminal2"] .terminal-window',
      )

      // Remove lizzy from terminal2
      const t2Window = terminal2Grid(page).locator('.terminal-window').first()
      await t2Window.locator('.tag-remove').click()
      await expect(t2Window.locator('.session-tag')).toHaveCount(0)

      // terminal1 jack should be unaffected
      await page.locator('.tab').filter({ hasText: /^Terminal$/ }).click()
      const t1Window = terminal1Grid(page).locator('.terminal-window').first()
      await expect(t1Window.locator('.tag-name')).toContainText('jack')
    })
  })
})
