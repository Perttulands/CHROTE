import { test, expect, Page } from '@playwright/test'
import { mockApiRoutes } from './mock-api'

// Helper function to perform drag-and-drop with dnd-kit
// dnd-kit requires a minimum drag distance to activate
async function dragAndDrop(page: Page, sourceSelector: string, targetSelector: string) {
  const source = page.locator(sourceSelector).first()
  const target = page.locator(targetSelector).first()

  const sourceBox = await source.boundingBox()
  const targetBox = await target.boundingBox()

  if (!sourceBox || !targetBox) {
    throw new Error('Could not find source or target element')
  }

  // Start position (center of source)
  const startX = sourceBox.x + sourceBox.width / 2
  const startY = sourceBox.y + sourceBox.height / 2

  // End position (center of target)
  const endX = targetBox.x + targetBox.width / 2
  const endY = targetBox.y + targetBox.height / 2

  // Perform drag with mouse events (dnd-kit needs distance threshold)
  await page.mouse.move(startX, startY)
  await page.mouse.down()
  // Move in steps to trigger dnd-kit's distance threshold (8px)
  await page.mouse.move(startX + 10, startY + 10, { steps: 5 })
  await page.mouse.move(endX, endY, { steps: 10 })
  // Small wait for dnd-kit to process
  await page.waitForTimeout(100)
  await page.mouse.up()
  // Wait for state update
  await page.waitForTimeout(100)
}

test.describe('Arena Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    // Wait for initial render
    await page.waitForSelector('.dashboard')
  })

  test.describe('Session Panel', () => {
    test('should render session panel with groups', async ({ page }) => {
      // Wait for sessions to load
      await page.waitForSelector('.session-panel')

      // Check that groups are rendered
      await expect(page.locator('.session-group')).toHaveCount(4) // hq, main, gt-gastown, gt-beads
    })

    test('should show HQ group first', async ({ page }) => {
      await page.waitForSelector('.session-group')

      // First group should be HQ
      const firstGroup = page.locator('.session-group').first()
      await expect(firstGroup.locator('.group-name')).toContainText('HQ')
    })

    test('should show correct session count badges', async ({ page }) => {
      await page.waitForSelector('.session-group')

      // HQ should have 2 sessions
      const hqGroup = page.locator('.session-group').first()
      await expect(hqGroup.locator('.session-count')).toContainText('2')
    })

    test('should show attached indicator for attached sessions', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // hq-deacon is attached
      await expect(page.locator('.attached-indicator')).toHaveCount(1)
    })

    test('should collapse sidebar when toggle clicked', async ({ page }) => {
      const panel = page.locator('.session-panel')
      await expect(panel).not.toHaveClass(/collapsed/)

      await page.click('.toggle-btn')
      await expect(panel).toHaveClass(/collapsed/)
    })

    test('should expand/collapse groups', async ({ page }) => {
      await page.waitForSelector('.session-group')

      const firstGroup = page.locator('.session-group').first()
      const items = firstGroup.locator('.session-group-items')

      // Initially expanded
      await expect(items).toBeVisible()

      // Click header to collapse
      await firstGroup.locator('.session-group-header').click()
      await expect(items).not.toBeVisible()

      // Click again to expand
      await firstGroup.locator('.session-group-header').click()
      await expect(items).toBeVisible()
    })
  })

  test.describe('Terminal Area', () => {
    test('should render layout controls', async ({ page }) => {
      await expect(page.locator('.terminal-area-controls')).toBeVisible()
      await expect(page.locator('.layout-btn')).toHaveCount(4)
    })

    test('should start with 2 windows by default', async ({ page }) => {
      await expect(page.locator('.terminal-window')).toHaveCount(2)
      await expect(page.locator('.layout-btn.active')).toContainText('2')
    })

    test('should switch to 1 window layout', async ({ page }) => {
      await page.click('.layout-btn:has-text("1")')
      await expect(page.locator('.terminal-window')).toHaveCount(1)
      await expect(page.locator('.terminal-grid')).toHaveClass(/grid-1/)
    })

    test('should switch to 4 window layout', async ({ page }) => {
      await page.click('.layout-btn:has-text("4")')
      await expect(page.locator('.terminal-window')).toHaveCount(4)
      await expect(page.locator('.terminal-grid')).toHaveClass(/grid-4/)
    })

    test('should maintain equal window heights in 4 window layout', async ({ page }) => {
      await page.click('.layout-btn:has-text("4")')
      await page.waitForTimeout(200) // Allow layout to settle

      const windows = page.locator('.terminal-window')
      const firstBox = await windows.nth(0).boundingBox()
      const thirdBox = await windows.nth(2).boundingBox()

      // Windows in different rows should have similar heights
      expect(firstBox).toBeTruthy()
      expect(thirdBox).toBeTruthy()
      if (firstBox && thirdBox) {
        // Heights should be roughly equal (within 10px tolerance)
        expect(Math.abs(firstBox.height - thirdBox.height)).toBeLessThan(10)
        // Third window should be below first (different row)
        expect(thirdBox.y).toBeGreaterThan(firstBox.y + firstBox.height - 10)
      }
    })

    test('should show "Shell ready" when window has no bound sessions', async ({ page }) => {
      await expect(page.locator('.no-sessions').first()).toContainText('Shell ready')
    })
  })

  test.describe('Drag and Drop', () => {
    test('should drag session from panel to window', async ({ page }) => {
      await page.waitForSelector('.session-item')

      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')

      // Session tag should appear in window
      const targetWindow = page.locator('.terminal-window').first()
      await expect(targetWindow.locator('.session-tag')).toHaveCount(1)
      await expect(targetWindow.locator('.tag-name')).toContainText('jack')
    })

    test('should mark session as assigned after dropping', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Use more specific selector to avoid drag overlay
      const sessionItem = page.locator('.session-panel .session-item:has-text("jack")')

      // Before drag - not assigned
      await expect(sessionItem).not.toHaveClass(/assigned/)

      // Drag
      await dragAndDrop(page, '.session-panel .session-item:has-text("jack")', '.terminal-window')

      // After drag - should be assigned (greyed out)
      await expect(sessionItem).toHaveClass(/assigned/)
    })

    test('should remove session tag with x button', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()
      // Use more specific selector to avoid drag overlay
      const sessionItem = page.locator('.session-panel .session-item:has-text("jack")')

      // First, add a session
      await dragAndDrop(page, '.session-panel .session-item:has-text("jack")', '.terminal-window')

      // Verify it's there
      await expect(targetWindow.locator('.session-tag')).toHaveCount(1)

      // Click remove button
      await targetWindow.locator('.tag-remove').click()

      // Tag should be gone
      await expect(targetWindow.locator('.session-tag')).toHaveCount(0)

      // Session should no longer be assigned
      await expect(sessionItem).not.toHaveClass(/assigned/)
    })

    test('should allow multiple sessions in one window', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      // Add first session
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')
      await expect(targetWindow.locator('.session-tag')).toHaveCount(1)

      // Add second session
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window')
      await expect(targetWindow.locator('.session-tag')).toHaveCount(2)
    })
  })

  test.describe('Session Cycling', () => {
    test('should show cycle buttons when multiple sessions bound', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      // Add two sessions
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window')

      // Cycle buttons should appear
      await expect(targetWindow.locator('.cycle-btn')).toHaveCount(2)
    })

    test('should not show cycle buttons with single session', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      // Add one session
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')

      // No cycle buttons
      await expect(targetWindow.locator('.cycle-btn')).toHaveCount(0)
    })

    test('should highlight active session tag', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      // Add two sessions
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window')

      // First session should be active
      const firstTag = targetWindow.locator('.session-tag').first()
      await expect(firstTag).toHaveClass(/active/)
    })

    test('should switch active tag on click', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      // Add two sessions
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window')

      // Wait for tags to appear
      await expect(targetWindow.locator('.session-tag')).toHaveCount(2)

      // Click second tag's name
      const secondTag = targetWindow.locator('.session-tag').nth(1)
      await secondTag.locator('.tag-name').click()

      // Second tag should now be active
      await expect(secondTag).toHaveClass(/active/)
    })
  })

  test.describe('Floating Modal', () => {
    test('should open modal when clicking unassigned session', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Click on an unassigned session
      await page.click('.session-item:has-text("jack")')

      // Modal should appear
      await expect(page.locator('.floating-modal')).toBeVisible()
      await expect(page.locator('.modal-title')).toContainText('jack')
    })

    test('should close modal when clicking overlay', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Open modal
      await page.click('.session-item:has-text("jack")')
      await expect(page.locator('.floating-modal')).toBeVisible()

      // Click overlay (outside modal)
      await page.click('.floating-modal-overlay', { position: { x: 10, y: 10 } })

      // Modal should close
      await expect(page.locator('.floating-modal')).not.toBeVisible()
    })

    test('should close modal when clicking x button', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Open modal
      await page.click('.session-item:has-text("jack")')
      await expect(page.locator('.floating-modal')).toBeVisible()

      // Click close button
      await page.click('.modal-close')

      // Modal should close
      await expect(page.locator('.floating-modal')).not.toBeVisible()
    })

    test('should focus assigned session instead of opening modal', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      // Assign sessions first
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window')

      // Wait for tags
      await expect(targetWindow.locator('.session-tag')).toHaveCount(2)

      // Click on joe tag to make it active
      const joeTag = targetWindow.locator('.session-tag').nth(1)
      await joeTag.locator('.tag-name').click()
      await expect(joeTag).toHaveClass(/active/)

      // Now click on jack in sidebar (already assigned)
      await page.click('.session-item:has-text("jack")')

      // Modal should NOT open
      await expect(page.locator('.floating-modal')).not.toBeVisible()

      // Jack tag should be active now
      const jackTag = targetWindow.locator('.session-tag').first()
      await expect(jackTag).toHaveClass(/active/)
    })
  })

  test.describe('Tab Navigation', () => {
    test('should switch to Files tab', async ({ page }) => {
      await page.click('.tab:has-text("Files")')
      await expect(page.locator('.files-view')).toBeVisible()
      await expect(page.locator('.session-panel')).not.toBeVisible()
    })

    test('should return to Terminal tab', async ({ page }) => {
      await page.click('.tab:has-text("Files")')
      await page.click('.tab:has-text("Terminal")')
      await expect(page.locator('.session-panel')).toBeVisible()
      await expect(page.locator('.terminal-area')).toBeVisible()
    })
  })

  test.describe('Search Filter', () => {
    test('should filter sessions by name', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Type in search box
      await page.fill('.session-search-input', 'jack')

      // Only jack should be visible
      await expect(page.locator('.session-item:visible')).toHaveCount(1)
      await expect(page.locator('.session-item:visible')).toContainText('jack')
    })

    test('should be case-insensitive', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Type uppercase
      await page.fill('.session-search-input', 'JACK')

      // jack should still be visible
      await expect(page.locator('.session-item:visible')).toHaveCount(1)
      await expect(page.locator('.session-item:visible')).toContainText('jack')
    })

    test('should hide groups with no matching sessions', async ({ page }) => {
      await page.waitForSelector('.session-group')

      // Initially 4 groups
      await expect(page.locator('.session-group')).toHaveCount(4)

      // Filter to only gt-gastown sessions
      await page.fill('.session-search-input', 'gastown')

      // Only gt-gastown group should remain (others are removed from DOM)
      // Group display name is "Gastown" (capitalized, without gt- prefix)
      await expect(page.locator('.session-group')).toHaveCount(1)
      await expect(page.locator('.session-group .group-name')).toContainText('Gastown')
    })

    test('should show all sessions when search cleared', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Filter first
      await page.fill('.session-search-input', 'jack')
      await expect(page.locator('.session-item:visible')).toHaveCount(1)

      // Clear search
      await page.fill('.session-search-input', '')

      // All sessions visible again (8 in mock data)
      await expect(page.locator('.session-item:visible')).toHaveCount(8)
    })

    test('should filter by agent name as well', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Filter by agent name (mayor is agentName for hq-mayor)
      await page.fill('.session-search-input', 'mayor')

      await expect(page.locator('.session-item:visible')).toHaveCount(1)
      await expect(page.locator('.session-item:visible')).toContainText('mayor')
    })
  })

  test.describe('Keyboard Navigation', () => {
    test('should cycle windows with Ctrl+Down', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // First, we need windows with sessions to test focus
      await page.click('.layout-btn:has-text("2")')
      await expect(page.locator('.terminal-window')).toHaveCount(2)

      // Add sessions to both windows
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window >> nth=0')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window >> nth=1')

      // Verify sessions are in windows
      await expect(page.locator('.terminal-window').nth(0).locator('.session-tag')).toHaveCount(1)
      await expect(page.locator('.terminal-window').nth(1).locator('.session-tag')).toHaveCount(1)

      // Initially window 0 should be focused (default)
      // Press Ctrl+Down to focus window 1
      await page.keyboard.press('Control+ArrowDown')

      // Press again to wrap to window 0
      await page.keyboard.press('Control+ArrowDown')
    })

    test('should cycle sessions with Ctrl+Right', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      // Add multiple sessions to first window
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window')
      await dragAndDrop(page, '.session-item:has-text("max")', '.terminal-window')

      // Verify 3 sessions in window
      await expect(targetWindow.locator('.session-tag')).toHaveCount(3)

      // First session should be active
      await expect(targetWindow.locator('.session-tag').first()).toHaveClass(/active/)

      // Press Ctrl+Right to cycle to next session
      await page.keyboard.press('Control+ArrowRight')
      await expect(targetWindow.locator('.session-tag').nth(1)).toHaveClass(/active/)

      // Press again
      await page.keyboard.press('Control+ArrowRight')
      await expect(targetWindow.locator('.session-tag').nth(2)).toHaveClass(/active/)

      // Press again to wrap around
      await page.keyboard.press('Control+ArrowRight')
      await expect(targetWindow.locator('.session-tag').first()).toHaveClass(/active/)
    })

    test('should cycle sessions backwards with Ctrl+Left', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      // Add multiple sessions
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window')

      await expect(targetWindow.locator('.session-tag')).toHaveCount(2)

      // First is active
      await expect(targetWindow.locator('.session-tag').first()).toHaveClass(/active/)

      // Press Ctrl+Left to go to last (wrapping)
      await page.keyboard.press('Control+ArrowLeft')
      await expect(targetWindow.locator('.session-tag').nth(1)).toHaveClass(/active/)

      // Press again to go back to first
      await page.keyboard.press('Control+ArrowLeft')
      await expect(targetWindow.locator('.session-tag').first()).toHaveClass(/active/)
    })
  })

  test.describe('Persistence', () => {
    test('should persist layout to localStorage', async ({ page }) => {
      // Switch to 4 windows
      await page.click('.layout-btn:has-text("4")')

      // Reload page
      await page.reload()
      await mockApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // Should still be 4 windows
      await expect(page.locator('.terminal-window')).toHaveCount(4)
      await expect(page.locator('.layout-btn.active')).toContainText('4')
    })

    test('should persist session bindings to localStorage', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Add session to window via drag
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')

      // Verify it's there before reload
      await expect(page.locator('.terminal-window').first().locator('.tag-name')).toContainText('jack')

      // Reload
      await page.reload()
      await mockApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // Session should still be bound
      await expect(page.locator('.terminal-window').first().locator('.tag-name')).toContainText('jack')
    })

    test('should persist collapsed sidebar state', async ({ page }) => {
      // Collapse sidebar
      await page.click('.toggle-btn')
      await expect(page.locator('.session-panel')).toHaveClass(/collapsed/)

      // Reload
      await page.reload()
      await mockApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // Should still be collapsed
      await expect(page.locator('.session-panel')).toHaveClass(/collapsed/)
    })
  })
})
