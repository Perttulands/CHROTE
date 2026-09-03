import { test, expect, Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { openSessionsSidecar } from './helpers'

// Presets live in the active terminal tab's own menu; the caret on the tab is
// its trigger, and there is no panel and no pseudo-tab.
async function openTabMenu(page: Page) {
  await page.locator('.tab.active .tab-menu-caret').click()
  await page.waitForSelector('.menu-sheet')
}

// Helper: save a preset by naming it in the row that offered it.
async function savePreset(page: Page, name: string) {
  await openTabMenu(page)
  await page.locator('.menu-row', { hasText: 'Save layout as preset' }).click()
  await page.fill('.menu-inline-input', name)
  await page.keyboard.press('Enter')
  await expect(page.locator('.menu-sheet')).toHaveCount(0)
}

// Helper: the presets the Restore submenu is currently offering.
function restoreRows(page: Page) {
  return page.locator('.menu-submenu .menu-row')
}

// Helper: drag a session into a window (simplified — uses mouse events for dnd-kit)
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

// Helper: seed localStorage with N presets so we can test the limit without saving 10 times via UI
function buildPresetJSON(count: number): string {
  const presets = Array.from({ length: count }, (_, i) => ({
    id: `preset-seed-${i}`,
    name: `Seed Preset ${i + 1}`,
    createdAt: Date.now() - (count - i) * 1000,
    workspaces: {
      terminal1: {
        windows: [
          { id: 'terminal1-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
        ],
        windowCount: 2,
      },
      terminal2: {
        windows: [
          { id: 'terminal2-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
          { id: 'terminal2-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
        ],
        windowCount: 2,
      },
    },
  }))
  return JSON.stringify(presets)
}

test.describe('Layout Presets', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    // Clear persisted dashboard state before first app boot without affecting
    // reloads later in the same test.
    await page.addInitScript(() => {
      const clearFlag = '__chrote_presets_storage_cleared'
      if (sessionStorage.getItem(clearFlag) === '1') return
      localStorage.clear()
      sessionStorage.setItem(clearFlag, '1')
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page)
  })

  test('names a preset in the menu and keeps it across a reload', async ({ page }) => {
    await openTabMenu(page)
    await page.locator('.menu-row', { hasText: 'Restore preset' }).click()
    await expect(restoreRows(page)).toHaveText(['No presets'])
    await page.keyboard.press('Escape')

    await savePreset(page, 'My Layout')

    await openTabMenu(page)
    await page.locator('.menu-row', { hasText: 'Restore preset' }).click()
    await expect(restoreRows(page)).toHaveText(['My Layout'])
    await page.keyboard.press('Escape')

    await page.reload()
    await openTabMenu(page)
    await page.locator('.menu-row', { hasText: 'Restore preset' }).click()
    await expect(restoreRows(page)).toHaveText(['My Layout'])
    await page.keyboard.press('Escape')

    // The limit is stated on the status line, where every announcement lands.
    await page.evaluate(json => localStorage.setItem('chrote-dashboard-presets', json), buildPresetJSON(10))
    await page.reload()
    await savePreset(page, 'One More')
    await expect(page.locator('.status-line')).toContainText('Maximum 10 presets reached')
  })

  test('restores a saved layout from the tab menu', async ({ page }) => {
    await page.waitForSelector('.session-item')

    // Bind a session to window 0
    await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window:visible >> nth=0')
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('jack')

    await savePreset(page, 'With Jack')

    // Replace the saved binding before loading so the preset must cleanly restore it.
    await page.locator('.terminal-window:visible').nth(0).locator('.tag-remove').click()
    const joeRow = page.locator('.session-item:has-text("joe")').first()
    await joeRow.getByRole('button', { name: /Session actions/ }).click()
    await page.getByRole('menuitem', { name: /Attach to window/ }).click()
    await page.locator('.menu-submenu').getByRole('menuitem', { name: 'Window 1', exact: true }).click()
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('joe')

    await openTabMenu(page)
    await page.locator('.menu-row', { hasText: 'Restore preset' }).click()
    await restoreRows(page).filter({ hasText: 'With Jack' }).click()

    // The menu closes behind the action it ran.
    await expect(page.locator('.menu-sheet')).toHaveCount(0)
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('jack')
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).not.toContainText('joe')
  })

  test('deletes a preset from the tab menu, confirming in the row', async ({ page }) => {
    await savePreset(page, 'First')
    await savePreset(page, 'Second')

    await openTabMenu(page)
    await page.locator('.menu-row', { hasText: 'Delete preset' }).click()
    await page.locator('.menu-submenu .menu-row', { hasText: 'First' }).click()
    // The first press arms the row; nothing is gone until the second.
    await page.locator('.menu-submenu .menu-row', { hasText: 'Confirm delete First' }).click()
    await expect(page.locator('.menu-sheet')).toHaveCount(0)

    await openTabMenu(page)
    await page.locator('.menu-row', { hasText: 'Restore preset' }).click()
    await expect(restoreRows(page)).toHaveText(['Second'])
  })

})
