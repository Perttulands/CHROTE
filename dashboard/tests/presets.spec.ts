import { test, expect, Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

// Helper: open the presets panel via the tab bar button
async function openPresetsPanel(page: Page) {
  await page.click('.tab[title="Layout Presets"]')
  await page.waitForSelector('.presets-panel')
}

// Helper: save a preset with the given name (panel must already be open)
async function savePreset(page: Page, name: string) {
  await page.fill('.preset-name-input', name)
  await page.click('.preset-save-btn')
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
  })

  test('should save current layout as preset', async ({ page }) => {
    // Open presets panel
    await openPresetsPanel(page)

    // Should show empty state initially
    await expect(page.locator('.preset-empty-message')).toBeVisible()

    // Save a preset
    await savePreset(page, 'My Layout')

    // Preset should appear in the list
    await expect(page.locator('.preset-item')).toHaveCount(1)
    await expect(page.locator('.preset-name')).toContainText('My Layout')

    // Empty state should be gone
    await expect(page.locator('.preset-empty-message')).not.toBeVisible()

    // The input should be cleared after save
    await expect(page.locator('.preset-name-input')).toHaveValue('')
  })

  test('should load preset and restore layout', async ({ page }) => {
    await page.waitForSelector('.session-item')

    // Bind a session to window 0
    await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window:visible >> nth=0')
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('jack')

    // Save the layout
    await openPresetsPanel(page)
    await savePreset(page, 'With Jack')
    await page.click('.presets-panel-close')

    // Change the layout: remove jack (click the remove button on the tag)
    await page.locator('.terminal-window:visible').nth(0).locator('.tag-remove').click()
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.session-tag')).toHaveCount(0)

    // Now load the saved preset
    await openPresetsPanel(page)
    await page.click('.preset-load-btn')

    // Panel should close after load
    await expect(page.locator('.presets-panel')).not.toBeVisible()

    // Jack should be restored in window 0
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('jack')
  })

  test('should delete preset from list', async ({ page }) => {
    // Save two presets
    await openPresetsPanel(page)
    await savePreset(page, 'First')
    await savePreset(page, 'Second')
    await expect(page.locator('.preset-item')).toHaveCount(2)

    // Delete the first one
    await page.locator('.preset-item').first().locator('.preset-delete-btn').click()

    // Only one should remain
    await expect(page.locator('.preset-item')).toHaveCount(1)
    await expect(page.locator('.preset-name')).toContainText('Second')
  })

  test('should rename preset', async ({ page }) => {
    // Save a preset
    await openPresetsPanel(page)
    await savePreset(page, 'Old Name')
    await expect(page.locator('.preset-name')).toContainText('Old Name')

    // Click the rename button (pencil icon)
    await page.click('.preset-rename-btn')

    // Edit form should appear with the current name
    await expect(page.locator('.preset-edit-input')).toBeVisible()
    await expect(page.locator('.preset-edit-input')).toHaveValue('Old Name')

    // Clear and type new name
    await page.locator('.preset-edit-input').fill('New Name')
    await page.click('.preset-edit-save')

    // Should show the new name
    await expect(page.locator('.preset-name')).toContainText('New Name')

    // Edit form should be gone
    await expect(page.locator('.preset-edit-input')).not.toBeVisible()
  })

  test('should enforce MAX_PRESETS limit', async ({ page }) => {
    // Seed localStorage with 10 presets (MAX_PRESETS), then reload
    const json = buildPresetJSON(10)
    await page.evaluate((j: string) => {
      localStorage.setItem('chrote-dashboard-presets', j)
    }, json)
    await page.reload()
    await page.waitForSelector('.dashboard')

    await openPresetsPanel(page)

    // Should show all 10 presets
    await expect(page.locator('.preset-item')).toHaveCount(10)

    // Warning should be visible
    await expect(page.locator('.preset-limit-warning')).toBeVisible()
    await expect(page.locator('.preset-limit-warning')).toContainText('Maximum 10 presets reached')

    // Save button should be disabled even with text entered
    await page.fill('.preset-name-input', 'One More')
    await expect(page.locator('.preset-save-btn')).toBeDisabled()
  })

  test('should persist presets across page reload', async ({ page }) => {
    // Save a preset
    await openPresetsPanel(page)
    await savePreset(page, 'Persistent Layout')
    await expect(page.locator('.preset-item')).toHaveCount(1)
    await page.click('.presets-panel-close')

    // Reload the page — API routes from beforeEach persist across reloads
    await page.reload()
    await page.waitForSelector('.dashboard')

    // Open presets panel again
    await openPresetsPanel(page)

    // Preset should still be there
    await expect(page.locator('.preset-item')).toHaveCount(1)
    await expect(page.locator('.preset-name')).toContainText('Persistent Layout')
  })

  test('should load preset when sessions already bound (clean replace)', async ({ page }) => {
    await page.waitForSelector('.session-item')

    // Bind joe to window 0
    await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window:visible >> nth=0')
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('joe')

    // Save this layout
    await openPresetsPanel(page)
    await savePreset(page, 'Joe Layout')
    await page.click('.presets-panel-close')

    // Now change: remove joe, add jack instead
    await page.locator('.terminal-window:visible').nth(0).locator('.tag-remove').click()
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.session-tag')).toHaveCount(0)
    const jackRow = page.locator('.session-item:has-text("jack")').first()
    await jackRow.getByRole('button', { name: /Session actions/ }).click()
    await page.getByRole('button', { name: /Attach to Window/ }).click()
    await page.locator('.session-context-submenu').getByRole('button', { name: 'Window 1', exact: true }).click()
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('jack')

    // Load the Joe preset — should cleanly replace the current layout
    await openPresetsPanel(page)
    await page.click('.preset-load-btn')

    // Joe should be back, jack should be gone
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('joe')
    // Jack should not appear in any window
    const allTags = page.locator('.terminal-window:visible .tag-name')
    const tagTexts = await allTags.allTextContents()
    expect(tagTexts.some(t => t.includes('jack'))).toBe(false)
  })

  test('should leave Ctrl+1 available to terminal input', async ({ page }) => {
    await page.waitForSelector('.session-item')

    // Bind jack to window 0
    await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window:visible >> nth=0')
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.tag-name')).toContainText('jack')

    // Save as first preset
    await openPresetsPanel(page)
    await savePreset(page, 'Shortcut Layout')
    await page.click('.presets-panel-close')

    // Change the layout: remove jack
    await page.locator('.terminal-window:visible').nth(0).locator('.tag-remove').click()
    await expect(page.locator('.terminal-window:visible').nth(0).locator('.session-tag')).toHaveCount(0)

    // Ctrl+1 is intentionally not a dashboard shortcut.
    // Use page.evaluate to dispatch the keyboard event on the main window
    // (page.keyboard.press may get intercepted by iframes)
    await page.evaluate(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', {
        key: '1',
        ctrlKey: true,
        bubbles: true,
      }))
    })

    await expect(page.locator('.terminal-window:visible').nth(0).locator('.session-tag')).toHaveCount(0)
  })
})
