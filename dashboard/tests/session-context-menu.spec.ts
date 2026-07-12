import { test, expect, Page } from './fixtures'
import { mockFileApiRoutes, mockSessions } from './mock-api'

/**
 * Helper: set up API mocks that also handle DELETE and PATCH (rename) for sessions.
 * After a delete, the deleted session is removed from subsequent GET responses.
 * After a rename, the old name is replaced in subsequent GET responses.
 */
async function mockApiRoutesWithMutations(page: Page) {
  await page.route(/.*\/terminal\/?.*/, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'text/html',
      body: '<html><body>mock terminal</body></html>',
    })
  })

  await page.route('**/api/tmux/appearance', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true }),
    })
  })

  await mockFileApiRoutes(page)

  // Mutable copy of session list so delete/rename are reflected on refresh
  let sessions = structuredClone(mockSessions.sessions)

  const buildResponse = () => {
    const grouped: Record<string, typeof sessions> = {}
    for (const s of sessions) {
      const g = s.group
      if (!grouped[g]) grouped[g] = []
      grouped[g].push(s)
    }
    return {
      sessions,
      grouped,
      timestamp: new Date().toISOString(),
    }
  }

  // GET /api/tmux/sessions
  await page.route('**/api/tmux/sessions', async (route, request) => {
    if (request.method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(buildResponse()),
      })
    } else {
      await route.continue()
    }
  })

  // DELETE /api/tmux/sessions/<name>
  await page.route('**/api/tmux/sessions/*', async (route, request) => {
    const url = request.url()
    const encodedName = url.split('/api/tmux/sessions/')[1]
    if (!encodedName) { await route.continue(); return }
    const sessionName = decodeURIComponent(encodedName)

    if (request.method() === 'DELETE') {
      sessions = sessions.filter(s => s.name !== sessionName)
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true }) })
    } else if (request.method() === 'PATCH') {
      const body = request.postDataJSON()
      const newName = body?.newName
      if (newName) {
        const idx = sessions.findIndex(s => s.name === sessionName)
        if (idx !== -1) sessions[idx] = { ...sessions[idx], name: newName }
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ success: true }) })
    } else {
      await route.continue()
    }
  })
}

// Helper: drag-and-drop for assigning sessions to windows (same as dashboard.spec.ts)
async function dragAndDrop(page: Page, sourceSelector: string, targetSelector: string) {
  const source = page.locator(sourceSelector).first().locator('.session-drag-handle')
  const target = page.locator(targetSelector).first()
  const sourceBox = await source.boundingBox()
  const targetBox = await target.boundingBox()
  if (!sourceBox || !targetBox) throw new Error('Could not find source or target element')
  const startX = sourceBox.x + sourceBox.width / 2
  const startY = sourceBox.y + sourceBox.height / 2
  const endX = targetBox.x + targetBox.width / 2
  const endY = targetBox.y + targetBox.height / 2
  await page.mouse.move(startX, startY)
  await page.mouse.down()
  await page.mouse.move(startX + 10, startY + 10, { steps: 5 })
  await page.mouse.move(endX, endY, { steps: 10 })
  // drag settle — no event to wait for
  await page.waitForTimeout(100)
  await page.mouse.up()
  // drag settle — no event to wait for
  await page.waitForTimeout(100)
}

test.describe('Session Context Menu', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutesWithMutations(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.waitForSelector('.session-item')
  })

  // -------------------------------------------------------
  // 1. Right-click opens context menu
  // -------------------------------------------------------
  test('right-click on session item opens context menu', async ({ page }) => {
    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    const menu = page.locator('.session-context-menu')
    await expect(menu).toBeVisible()

    // Menu should contain standard items
    await expect(menu.locator('.session-context-item:has-text("Rename")')).toBeVisible()
    await expect(menu.locator('.session-context-item:has-text("Kill Session")')).toBeVisible()
    await expect(menu.locator('.session-context-item:has-text("Attach to Window")')).toBeVisible()
  })

  // -------------------------------------------------------
  // 2. Rename flow: context menu -> Rename -> type new name -> Enter -> verify
  // -------------------------------------------------------
  test('rename session via context menu and Enter', async ({ page }) => {
    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    // Click Rename
    await page.locator('.session-context-menu .session-context-item:has-text("Rename")').click()

    // Context menu should close, rename input should appear
    await expect(page.locator('.session-context-menu')).not.toBeVisible()
    const input = page.locator('.session-rename-input')
    await expect(input).toBeVisible()
    await expect(input).toBeFocused()

    // The input should be pre-filled with the current name
    await expect(input).toHaveValue('hq-mayor')

    // Clear and type new name
    await input.fill('hq-commander')
    await input.press('Enter')

    // Rename input should disappear
    await expect(input).not.toBeVisible()

    // After the API call + refresh, the session should appear with new name
    await expect(page.locator('.session-item:has-text("hq-commander")')).toBeVisible()
    await expect(page.locator('.session-item:has-text("hq-mayor")')).not.toBeVisible()
  })

  // -------------------------------------------------------
  // 3. Rename cancel via Escape reverts name
  // -------------------------------------------------------
  test('rename cancel via Escape reverts name', async ({ page }) => {
    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    await page.locator('.session-context-menu .session-context-item:has-text("Rename")').click()

    const input = page.locator('.session-rename-input')
    await expect(input).toBeVisible()

    // Type a different name but press Escape
    await input.fill('something-else')
    await input.press('Escape')

    // Input should disappear and original name should remain
    await expect(input).not.toBeVisible()
    await expect(page.locator('.session-item:has-text("hq-mayor")')).toBeVisible()
    await expect(page.locator('.session-item:has-text("something-else")')).not.toBeVisible()
  })

  // -------------------------------------------------------
  // 4. Rename with empty string is rejected
  // -------------------------------------------------------
  test('rename with empty string is rejected', async ({ page }) => {
    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    await page.locator('.session-context-menu .session-context-item:has-text("Rename")').click()

    const input = page.locator('.session-rename-input')
    await expect(input).toBeVisible()

    // Clear the input entirely and submit
    await input.fill('')
    await input.press('Enter')

    // The rename should be a no-op (empty string rejected by handleRenameSubmit guard).
    // After the blur/submit, the original name should still be there.
    await expect(page.locator('.session-item:has-text("hq-mayor")')).toBeVisible()
  })

  // -------------------------------------------------------
  // 5. Kill session via context menu
  // -------------------------------------------------------
  test('kill session via context menu removes it from list', async ({ page }) => {
    // Verify it exists first
    await expect(page.locator('.session-item:has-text("hq-mayor")')).toBeVisible()

    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    // Click Kill
    await page.locator('.session-context-menu .session-context-item:has-text("Kill Session")').click()

    // Session should be removed after the API call + refresh
    await expect(page.locator('.session-item:has-text("hq-mayor")')).not.toBeVisible()
  })

  // -------------------------------------------------------
  // 6. "Attach to Window" submenu shows correct windows
  // -------------------------------------------------------
  test('attach to window submenu shows correct windows', async ({ page }) => {
    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    const menu = page.locator('.session-context-menu')
    await expect(menu).toBeVisible()

    // Click "Attach to Window" to reveal the keyboard/touch-operable submenu
    const assignTrigger = menu.getByRole('button', { name: /Attach to Window/ })
    await assignTrigger.click()

    // Submenu should appear with window entries
    const submenu = page.locator('.session-context-submenu')
    await expect(submenu).toBeVisible()

    // Default layout is 2 windows per workspace (terminal1 + terminal2).
    // Should have entries like "Window 1", "Window 2", "Terminal 2 - Window 1", "Terminal 2 - Window 2"
    const windowItems = submenu.locator('.session-context-item')
    const count = await windowItems.count()
    expect(count).toBeGreaterThanOrEqual(2)

    // At minimum, "Window 1" should be present (use exact text to avoid matching "Terminal 2 - Window 1")
    await expect(submenu.getByRole('button', { name: 'Window 1', exact: true })).toBeVisible()
  })

  // -------------------------------------------------------
  // 7. "Unassign" option appears only for assigned sessions
  // -------------------------------------------------------
  test('unassign option only appears for assigned sessions', async ({ page }) => {
    // Right-click on an unassigned session
    const unassigned = page.locator('.session-item:has-text("gt-gastown-jack")')
    await unassigned.click({ button: 'right' })

    let menu = page.locator('.session-context-menu')
    await expect(menu).toBeVisible()

    // Unassign should NOT be visible for unassigned sessions
    await expect(menu.locator('.session-context-item:has-text("Unassign")')).not.toBeVisible()

    // Close menu
    await page.click('body', { position: { x: 1, y: 1 } })
    await expect(menu).not.toBeVisible()

    // Now assign the session via drag-and-drop
    await dragAndDrop(page, '.session-panel .session-item:has-text("gt-gastown-jack")', '.terminal-window')

    // Verify it's assigned
    const sessionItem = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    await expect(sessionItem).toHaveClass(/assigned/)

    // Right-click the now-assigned session
    await sessionItem.click({ button: 'right' })

    menu = page.locator('.session-context-menu')
    await expect(menu).toBeVisible()

    // Unassign SHOULD now be visible
    await expect(menu.locator('.session-context-item:has-text("Unassign")')).toBeVisible()
  })

  // -------------------------------------------------------
  // 8. Context menu closes on click outside
  // -------------------------------------------------------
  test('context menu closes on click outside', async ({ page }) => {
    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    const menu = page.locator('.session-context-menu')
    await expect(menu).toBeVisible()

    // Click somewhere outside the menu
    await page.click('body', { position: { x: 1, y: 1 } })

    // Menu should be gone
    await expect(menu).not.toBeVisible()
  })

  // -------------------------------------------------------
  // 9. Mobile long-press opens context menu
  // -------------------------------------------------------
  test('mobile long-press (500ms) opens context menu', async ({ page }) => {
    const session = page.locator('.session-item:has-text("hq-mayor")')
    const box = await session.boundingBox()
    if (!box) throw new Error('Session item not found')

    const cx = box.x + box.width / 2
    const cy = box.y + box.height / 2

    // Simulate touch-based long press via dispatchEvent (Playwright doesn't natively
    // support touchstart/touchend, so we dispatch them on the element directly).
    await page.evaluate(({ x, y }) => {
      const el = document.elementFromPoint(x, y)
      if (!el) return

      const touch = new Touch({
        identifier: 1,
        target: el,
        clientX: x,
        clientY: y,
        pageX: x,
        pageY: y,
      })

      el.dispatchEvent(new TouchEvent('touchstart', {
        touches: [touch],
        targetTouches: [touch],
        changedTouches: [touch],
        bubbles: true,
        cancelable: true,
      }))
    }, { x: cx, y: cy })

    // Must exceed the 500ms long-press threshold — no DOM event to await
    await page.waitForTimeout(600)

    // Dispatch touchend to clean up
    await page.evaluate(({ x, y }) => {
      const el = document.elementFromPoint(x, y)
      if (!el) return

      const touch = new Touch({
        identifier: 1,
        target: el,
        clientX: x,
        clientY: y,
        pageX: x,
        pageY: y,
      })

      el.dispatchEvent(new TouchEvent('touchend', {
        touches: [],
        targetTouches: [],
        changedTouches: [touch],
        bubbles: true,
        cancelable: true,
      }))
    }, { x: cx, y: cy })

    // Context menu should now be visible
    const menu = page.locator('.session-context-menu')
    await expect(menu).toBeVisible()
  })
})
