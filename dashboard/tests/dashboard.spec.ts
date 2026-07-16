import { test, expect, Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { openSessionsSidecar } from './helpers'

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
  // drag settle — no event to wait for
  await page.waitForTimeout(100)
  await page.mouse.up()
  // drag settle — no event to wait for
  await page.waitForTimeout(100)
}

test.describe('Arena Dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    // Wait for initial render
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page)
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

    test('does not show a decorative attached indicator without a dashboard assignment', async ({ page }) => {
      await page.waitForSelector('.session-item')

      await expect(page.locator('.attached-indicator')).toHaveCount(0)
      await expect(page.locator('.window-location-chip')).toHaveCount(0)
    })

    test('closes the active sidecar from its stable trigger and reopens it as an overlay', async ({ page }) => {
      const panel = page.locator('.session-panel')
      await expect(panel).toHaveClass(/sidecar-pinned/)

      await page.getByRole('button', { name: 'Sessions sidecar', exact: true }).click()
      await expect(panel).toHaveCount(0)

      await page.getByRole('button', { name: 'Sessions sidecar', exact: true }).click()
      await expect(page.locator('.session-panel')).toHaveClass(/sidecar-overlay/)
    })

    test('switches one pinned sidecar between Sessions and Files and opens one non-modal file Peek', async ({ page }) => {
      await page.route(/.*\/api\/files\/resources(?:\/.*)?$/, async route => {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            isDir: true,
            items: [{ name: 'readme.txt', path: '/readme.txt', isDir: false, size: 12, modified: '2026-07-13T00:00:00Z', type: 'text/plain' }],
          }),
        })
      })
      await expect(page.locator('.session-panel')).toHaveClass(/sidecar-pinned/)
      await expect(page.locator('.terminal-files-panel')).toHaveCount(0)

      await dragAndDrop(page, '.session-item:has-text("hq-mayor")', '.terminal-window')
      const terminalFrame = page.locator('iframe[title^="Terminal -"]').first()
      await expect(terminalFrame).toBeAttached()
      await terminalFrame.evaluate(element => { element.setAttribute('data-dock-identity', 'preserved') })

      await page.getByRole('button', { name: 'Files sidecar', exact: true }).click()
      const files = page.locator('.terminal-files-panel')
      await expect(page.locator('.session-panel')).toHaveCount(0)
      await expect(files).toHaveClass(/sidecar-pinned/)

      await page.getByRole('treeitem', { name: /File readme\.txt/ }).click()
      const peek = page.getByRole('dialog', { name: /File Peek: readme\.txt/ })
      await expect(peek).toBeVisible()
      await expect(page.locator('.file-peek-overlay')).toHaveCount(0)
      await page.getByRole('button', { name: 'Close file Peek' }).click()
      await expect(peek).toHaveCount(0)

      await page.getByRole('button', { name: 'Files sidecar', exact: true }).click()
      await expect(files).toHaveCount(0)
      await expect(page.locator('.session-panel')).toHaveCount(0)
      await expect(terminalFrame).toHaveAttribute('data-dock-identity', 'preserved')
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
      await expect(page.locator('.terminal-area-controls:visible')).toBeVisible()
      await expect(page.locator('.layout-btn:visible')).toHaveCount(6)
      await expect(page.getByRole('button', { name: 'Refit terminal layout' })).toBeVisible()
      await expect(page.getByRole('button', { name: 'Terminal recovery actions' })).toBeVisible()
    })

    test('should start with 2 windows by default', async ({ page }) => {
      await expect(page.locator('.terminal-window:visible')).toHaveCount(2)
      await expect(page.locator('.layout-btn.active:visible')).toContainText('2')
    })

    test('should switch to 1 window layout', async ({ page }) => {
      await page.click('.layout-btn:visible:has-text("1")')
      await expect(page.locator('.terminal-window:visible')).toHaveCount(1)
      await expect(page.locator('.terminal-grid:visible')).toHaveClass(/grid-1/)
    })

    test('should switch to 4 window layout', async ({ page }) => {
      await page.click('.layout-btn:visible:has-text("4")')
      await expect(page.locator('.terminal-window:visible')).toHaveCount(4)
      await expect(page.locator('.terminal-grid:visible')).toHaveClass(/grid-4/)
    })

    test('should maintain equal window heights in 4 window layout', async ({ page }) => {
      await page.click('.terminal-area:visible .layout-btn:has-text("4")')
      // Wait for all 4 windows to render instead of arbitrary timeout
      await expect(page.locator('.terminal-window:visible')).toHaveCount(4, { timeout: 3000 })

      const windows = page.locator('.terminal-window:visible')
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

    test('should show "New Session" button when window has no bound sessions', async ({ page }) => {
      await expect(page.locator('.empty-window-state').first()).toBeVisible()
      await expect(page.locator('.create-session-btn').first()).toContainText('New Session')
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

      // The most recently attached session is active.
      await expect(targetWindow.locator('.session-tag').last()).toHaveClass(/active/)
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

    test('opens Peek for an assigned row and focuses only from its location chip', async ({ page }) => {
      await page.waitForSelector('.session-item')

      const targetWindow = page.locator('.terminal-window').first()

      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window')
      await expect(targetWindow.locator('.session-tag')).toHaveCount(2)

      const joeTag = targetWindow.locator('.session-tag').nth(1)
      await joeTag.locator('.tag-name').click()
      await expect(joeTag).toHaveClass(/active/)

      const jackRow = page.locator('.session-item:has-text("jack")')
      await jackRow.locator('.session-name').click()
      await expect(page.locator('.floating-modal')).toBeVisible()
      await expect(joeTag).toHaveClass(/active/)

      await page.keyboard.press('Escape')
      await expect(page.locator('.floating-modal')).toHaveCount(0)
      await jackRow.locator('.window-location-chip').click()
      await expect(targetWindow.locator('.session-tag').first()).toHaveClass(/active/)
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
      await expect(page.locator('.terminal-area:visible')).toBeVisible()
    })

    test('keeps the mobile navigation clickable above its outside-dismiss layer', async ({ page }) => {
      await page.setViewportSize({ width: 700, height: 760 })
      await page.getByRole('button', { name: '☰' }).click()

      const menu = page.locator('.mobile-nav-dropdown')
      const layer = page.locator('.floating-panel-dismiss-layer')
      await expect(menu).toBeVisible()
      await expect(menu).toHaveCSS('position', 'absolute')
      expect(await menu.evaluate(element => Number(getComputedStyle(element).zIndex)))
        .toBeGreaterThan(await layer.evaluate(element => Number(getComputedStyle(element).zIndex)))

      await menu.getByRole('button', { name: 'Terminal 2' }).click()
      await expect(page.locator('.terminal-grid[data-workspace="terminal2"]')).toBeVisible()
      await expect(layer).toHaveCount(0)
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

    test('should filter by session name', async ({ page }) => {
      await page.waitForSelector('.session-item')

      // Filter by session name
      await page.fill('.session-search-input', 'hq-mayor')

      await expect(page.locator('.session-item:visible')).toHaveCount(1)
      await expect(page.locator('.session-item:visible')).toContainText('hq-mayor')
    })
  })

  test.describe('Terminal-safe keyboard handling', () => {
    test('does not hijack Ctrl+Arrow terminal input', async ({ page }) => {
      await page.waitForSelector('.session-item')
      const targetWindow = page.locator('.terminal-window:visible').first()
      await dragAndDrop(page, '.session-item:has-text("jack")', '.terminal-window:visible')
      await dragAndDrop(page, '.session-item:has-text("joe")', '.terminal-window:visible')
      await expect(targetWindow.locator('.session-tag')).toHaveCount(2)
      await expect(targetWindow.locator('.session-tag').last()).toHaveClass(/active/)
      await page.evaluate(() => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', ctrlKey: true, bubbles: true }))
      })
      await expect(targetWindow.locator('.session-tag').last()).toHaveClass(/active/)
    })
  })

  test.describe('Persistence', () => {
    test('should persist layout to localStorage', async ({ page }) => {
      // Switch to 4 windows
      await page.click('.layout-btn:visible:has-text("4")')

      // Reload page
      await page.reload()
      await mockApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // Should still be 4 windows
      await expect(page.locator('.terminal-window:visible')).toHaveCount(4)
      await expect(page.locator('.layout-btn.active:visible')).toContainText('4')
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

    test('persists a closed workspace sidecar', async ({ page }) => {
      await page.getByRole('button', { name: 'Sessions sidecar', exact: true }).click()
      await expect(page.locator('.session-panel')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Sessions sidecar', exact: true })).toHaveAttribute('aria-expanded', 'false')

      await page.reload()
      await mockApiRoutes(page)
      await page.waitForSelector('.dashboard')

      await expect(page.locator('.session-panel')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Sessions sidecar', exact: true })).toHaveAttribute('aria-expanded', 'false')
    })
  })
})
