import { test, expect, Page } from './fixtures'
import { mockFileApiRoutes, mockLaunchApiRoute, mockSessions, mockTerminalSocket, mockThemeApiRoute } from './mock-api'
import { openSessionsSidecar } from './helpers'

/**
 * Helper: set up API mocks that also handle DELETE and PATCH (rename) for sessions.
 * After a delete, the deleted session is removed from subsequent GET responses.
 * After a rename, the old name is replaced in subsequent GET responses.
 */
async function mockApiRoutesWithMutations(page: Page) {
  await mockTerminalSocket(page)

  await mockThemeApiRoute(page)
  await mockLaunchApiRoute(page)

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
  const source = page.locator(sourceSelector).first()
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
    await openSessionsSidecar(page)
    await page.waitForSelector('.session-item')
  })

  // -------------------------------------------------------
  // 1. Right-click opens context menu
  // -------------------------------------------------------
  test('right-click on session item opens context menu', async ({ page }) => {
    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    const menu = page.locator('.menu-sheet')
    await expect(menu).toBeVisible()

    // Menu should contain standard items
    await expect(menu.locator('.menu-row:has-text("Rename")')).toBeVisible()
    await expect(menu.locator('.menu-row:has-text("Kill session")')).toBeVisible()
    await expect(menu.locator('.menu-row:has-text("Attach to window")')).toBeVisible()
  })

  // -------------------------------------------------------
  // 2. Rename flow: context menu -> Rename -> type new name -> Enter -> verify
  // -------------------------------------------------------

  // -------------------------------------------------------
  // 3. Rename from the attached session tag menu
  // -------------------------------------------------------
  test('rename an attached session from its terminal header tag menu', async ({ page }) => {
    await dragAndDrop(page, '.session-panel .session-item:has-text("hq-mayor")', '.terminal-window')

    const window = page.locator('.terminal-window').first()
    const tag = window.locator('.session-tag:has-text("hq-mayor")')
    await expect(tag).toBeVisible()
    await tag.click({ button: 'right' })

    const menu = page.getByRole('menu', { name: 'Session actions for hq-mayor' })
    await expect(menu.getByRole('menuitem', { name: 'Rename session' })).toBeVisible()
    await menu.getByRole('menuitem', { name: 'Rename session' }).click()

    const input = window.getByRole('textbox', { name: 'Rename session hq-mayor' })
    await expect(input).toBeFocused()
    await page.evaluate(() => new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
    await expect(input).toBeFocused()
    await expect(input).toHaveValue('hq-mayor')
    await input.fill('hq-commander')
    await input.press('Enter')

    await expect(window.locator('.session-tag:has-text("hq-commander")')).toBeVisible()
    await expect(window.locator('.session-tag:has-text("hq-mayor")')).not.toBeVisible()
  })

  // -------------------------------------------------------
  // 4. Rename cancel via Escape reverts name
  // -------------------------------------------------------

  // -------------------------------------------------------
  // 4. Rename with empty string is rejected
  // -------------------------------------------------------

  // -------------------------------------------------------
  // 5. Kill session via context menu
  // -------------------------------------------------------
  test('kill session via context menu removes it from list', async ({ page }) => {
    // Verify it exists first
    await expect(page.locator('.session-item:has-text("hq-mayor")')).toBeVisible()

    const session = page.locator('.session-item:has-text("hq-mayor")')
    await session.click({ button: 'right' })

    // Kill confirms where it was chosen: the row arms, then it runs.
    await page.locator('.menu-sheet .menu-row:has-text("Kill session")').click()
    await page.locator('.menu-sheet .menu-row:has-text("Confirm kill")').click()

    // Session should be removed after the API call + refresh
    await expect(page.locator('.session-item:has-text("hq-mayor")')).not.toBeVisible()
  })

  // -------------------------------------------------------
  // 6. "Attach to window" submenu shows correct windows
  // -------------------------------------------------------

  // -------------------------------------------------------
  // 7. "Unassign" option appears only for assigned sessions
  // -------------------------------------------------------

  // -------------------------------------------------------
  // 8. Context menu closes on click outside
  // -------------------------------------------------------

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
    const menu = page.locator('.menu-sheet')
    await expect(menu).toBeVisible()
  })
})
