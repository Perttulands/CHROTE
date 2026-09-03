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

    test('shares Sessions across terminal tabs while Files follows its terminal workspace', async ({ page }) => {
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
      await page.evaluate(() => localStorage.setItem('chrote-files-persist-tab-state', '0'))
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

  test.describe('Terminal Area', () => {

    // The counts left the strip: the layout grows and shrinks on the chords the
    // strip names, and the strip states the number it reached.
    test('grows and shrinks the layout on its chords', async ({ page }) => {
      const controls = page.locator('.terminal-area:visible .terminal-area-controls')
      await expect(controls).toContainText('Alt+= add window · Alt+- remove empty')
      await expect(controls.locator('.layout-count')).toHaveText('2')
      await expect(controls.locator('.layout-btn')).toHaveCount(0)

      await page.keyboard.press('Alt+=')
      await page.keyboard.press('Alt+=')
      const windows = page.locator('.terminal-window:visible')
      await expect(windows).toHaveCount(4)
      await expect(page.locator('.terminal-grid:visible')).toHaveClass(/grid-4/)
      await expect(controls.locator('.layout-count')).toHaveText('4')

      const firstBox = await windows.nth(0).boundingBox()
      const thirdBox = await windows.nth(2).boundingBox()
      expect(firstBox).toBeTruthy()
      expect(thirdBox).toBeTruthy()
      expect(Math.abs(firstBox!.height - thirdBox!.height)).toBeLessThan(10)

      for (let press = 0; press < 3; press += 1) await page.keyboard.press('Alt+-')
      await expect(page.locator('.terminal-window:visible')).toHaveCount(1)
      await expect(page.locator('.terminal-grid:visible')).toHaveClass(/grid-1/)
      await expect(controls.locator('.layout-count')).toHaveText('1')
    })

    // Every control the cut removed, in one place: none of them is reachable,
    // and the two that survived moved into the active tab's own menu.
    test('offers the cut controls nowhere and the moved ones in the tab menu', async ({ page }) => {
      await openSessionsSidecar(page)

      await expect(page.locator('.cycle-btn')).toHaveCount(0)
      await expect(page.locator('.layout-btn')).toHaveCount(0)
      await expect(page.locator('.terminal-maintenance-btn')).toHaveCount(0)
      await expect(page.locator('.window-location-chip')).toHaveCount(0)
      await expect(page.locator('.session-panel .refresh-btn')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Pin Sessions sidecar' })).toHaveCount(0)
      await expect(page.getByRole('button', { name: '⋯ Tab' })).toHaveCount(0)
      await expect(page.getByRole('button', { name: /Refit/ })).toHaveCount(0)

      await page.locator('.tab.active .tab-menu-caret').click()
      const menu = page.locator('.menu-sheet')
      for (const item of ['Rename tab', 'Save layout as preset', 'Restore preset', 'Clear tab assignments', 'Reconnect frames', 'Claim all', 'Pin sessions panel']) {
        await expect(menu.locator('.menu-row', { hasText: item })).toHaveCount(1)
      }

      // The pin is a setting now, and the menu reads it back.
      await expect(menu.locator('.menu-row', { hasText: 'Pin sessions panel' })).toContainText('off')
      await menu.locator('.menu-row', { hasText: 'Pin sessions panel' }).click()
      await page.locator('.tab.active .tab-menu-caret').click()
      await expect(page.locator('.menu-sheet').locator('.menu-row', { hasText: 'Pin sessions panel' })).toContainText('on')
    })

  })

  test.describe('Session Cycling', () => {

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

  test.describe('Peek', () => {
    test('opens as a left sheet on an unassigned session and closes from its header', async ({ page }) => {
      await page.waitForSelector('.session-item')

      await page.click('.session-item:has-text("jack")')

      const peek = page.locator('.sheet.sheet-left')
      await expect(peek).toBeVisible()
      await expect(peek.locator('.peek-name')).toContainText('jack')

      await peek.getByRole('button', { name: 'Close Peek' }).click()
      await expect(peek).not.toBeVisible()

      // No backdrop: a click outside is a click outside, and Escape is the way out.
      await page.click('.session-item:has-text("jack")')
      await expect(peek).toBeVisible()
      await page.keyboard.press('Escape')
      await expect(peek).not.toBeVisible()
    })

  })

  test.describe('Search Filter', () => {
    test('should filter sessions by name', async ({ page }) => {
      await page.waitForSelector('.session-item')

      await page.fill('.session-search-input', 'JACK')

      await expect(page.locator('.session-item:visible')).toHaveCount(1)
      await expect(page.locator('.session-item:visible')).toContainText('jack')

      await page.fill('.session-search-input', 'gastown')
      await expect(page.locator('.session-group')).toHaveCount(1)
      await expect(page.locator('.session-group .group-name')).toContainText('gt-gastown')

      await page.fill('.session-search-input', '')
      await expect(page.locator('.session-item:visible')).toHaveCount(8)
    })

  })

})
