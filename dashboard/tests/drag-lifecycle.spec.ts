import { expect, test, type Locator, type Page } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'

const dragSessions = {
  ...mockSessions,
  sessions: mockSessions.sessions.map(session => (
    session.name === 'gt-gastown-jack' ? { ...session, unixUser: 'alice' } : session
  )),
  grouped: Object.fromEntries(Object.entries(mockSessions.grouped).map(([group, sessions]) => [
    group,
    sessions.map(session => (
      session.name === 'gt-gastown-jack' ? { ...session, unixUser: 'alice' } : session
    )),
  ])),
}

async function point(locator: Locator) {
  const box = await locator.boundingBox()
  if (!box) throw new Error('drag element has no bounding box')
  return { x: box.x + box.width / 2, y: box.y + box.height / 2 }
}

async function startMouseDrag(page: Page, handle: Locator, target: Locator) {
  const from = await point(handle)
  const to = await point(target)
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  await page.mouse.move(from.x + 12, from.y + 12, { steps: 4 })
  await page.mouse.move(to.x, to.y, { steps: 8 })
  await expect(page.locator('.dashboard')).toHaveClass(/is-dragging/)
}

async function finishMouseDrag(page: Page) {
  await page.mouse.up()
  await expect(page.locator('.dashboard')).not.toHaveClass(/is-dragging/)
}

async function startTouchPointerDrag(page: Page, handle: Locator, target: Locator) {
  const from = await point(handle)
  const to = await point(target)
  const pointerId = 41

  await handle.dispatchEvent('pointerdown', {
    bubbles: true,
    cancelable: true,
    pointerId,
    pointerType: 'touch',
    isPrimary: true,
    button: 0,
    buttons: 1,
    clientX: from.x,
    clientY: from.y,
  })
  await page.evaluate(({ x, y, pointerId }) => {
    document.dispatchEvent(new PointerEvent('pointermove', {
      bubbles: true,
      cancelable: true,
      pointerId,
      pointerType: 'touch',
      isPrimary: true,
      button: -1,
      buttons: 1,
      clientX: x,
      clientY: y,
    }))
  }, { ...to, pointerId })

  await expect(page.locator('.dashboard')).toHaveClass(/is-dragging/)
  return pointerId
}

async function cancelTouchPointerDrag(page: Page, pointerId: number) {
  await page.evaluate((id) => {
    document.dispatchEvent(new PointerEvent('pointercancel', {
      bubbles: true,
      cancelable: true,
      pointerId: id,
      pointerType: 'touch',
      isPrimary: true,
    }))
  }, pointerId)
  await expect(page.locator('.dashboard')).not.toHaveClass(/is-dragging/)
}

async function expectTagAssignment(firstWindow: Locator, secondWindow: Locator, firstCount: number, secondCount: number) {
  await expect(firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')).toHaveCount(firstCount)
  await expect(secondWindow.locator('.session-tag:has-text("gt-gastown-jack")')).toHaveCount(secondCount)
}

test.describe('terminal drag lifecycle', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page, { sessionsResponse: dragSessions })
    await page.goto('/')
    await page.waitForSelector('.session-panel .session-item')
  })

  test('only the overlay moves mid-drag and drop feedback has an opaque base', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const handle = row.locator('.session-drag-handle')
    const target = page.locator('.terminal-window:visible').first().locator('.terminal-window-body')
    const initialBox = await row.boundingBox()

    await expect(handle).toHaveCSS('touch-action', 'none')
    await startMouseDrag(page, handle, target)

    await expect(row).toHaveClass(/dragging/)
    await expect(row).toHaveCSS('opacity', '0')
    await expect(row).toHaveCSS('transform', 'none')
    await expect(row).toHaveCSS('transition-property', 'none')
    expect(await row.boundingBox()).toEqual(initialBox)

    const overlayWrapper = page.locator('.drag-overlay-wrapper')
    await expect(overlayWrapper).toBeVisible()
    await expect(overlayWrapper).toHaveCSS('pointer-events', 'none')
    await expect(page.locator('.dragging-overlay')).toHaveCount(1)

    await expect(page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-drop-overlay')).toHaveCount(2)
    await expect(page.locator('.terminal-grid[data-workspace="terminal2"] .terminal-drop-overlay')).toHaveCount(0)
    await expect(page.locator('.terminal-grid[data-workspace="terminal3"] .terminal-drop-overlay')).toHaveCount(0)

    const targetOverlay = target.locator('.terminal-drop-overlay')
    const bodyBox = await target.boundingBox()
    const dropBox = await targetOverlay.boundingBox()
    expect(dropBox).toEqual(bodyBox)
    const computedColors = await targetOverlay.evaluate((element) => {
      const probe = document.createElement('div')
      probe.style.backgroundColor = 'var(--surface-primary)'
      element.appendChild(probe)
      const semanticSurface = getComputedStyle(probe).backgroundColor
      probe.remove()
      return {
        overlay: getComputedStyle(element).backgroundColor,
        semanticSurface,
      }
    })
    expect(computedColors.overlay).toBe(computedColors.semanticSurface)
    expect(await targetOverlay.evaluate(element => getComputedStyle(element).backgroundImage)).toContain('linear-gradient')

    await page.keyboard.press('Escape')
    await page.mouse.up()

    await expect(page.locator('.dashboard')).not.toHaveClass(/is-dragging/)
    await expect(page.locator('.dragging-overlay')).toHaveCount(0)
    await expect(page.locator('.terminal-drop-overlay')).toHaveCount(0)
    await expect(row).not.toHaveClass(/dragging/)
    await expect(row).toHaveCSS('opacity', '1')
  })

  test('sub-8px mouse movement stays inactive and a real touch pointer drag activates only from the handle', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const handle = row.locator('.session-drag-handle')
    const from = await point(handle)

    await page.mouse.move(from.x, from.y)
    await page.mouse.down()
    await page.mouse.move(from.x + 7, from.y)
    await expect(page.locator('.dashboard')).not.toHaveClass(/is-dragging/)
    await expect(page.locator('.dragging-overlay')).toHaveCount(0)
    await page.mouse.up()

    const pointerId = await startTouchPointerDrag(page, handle, page.locator('.tab-bar'))
    await expect(row).toHaveClass(/dragging/)
    await expect(page.locator('.dragging-overlay')).toHaveCount(1)
    await cancelTouchPointerDrag(page, pointerId)
    await expect(row).not.toHaveClass(/dragging/)
  })

  test('non-interactive grip clicks stay inert while row click and right-click behavior remain available', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const handle = row.locator('.session-drag-handle')

    await expect(handle).toHaveJSProperty('tagName', 'SPAN')
    await expect(handle).toHaveAttribute('aria-hidden', 'true')
    await expect(handle).toHaveAttribute('title', 'Drag gt-gastown-jack (Unix user alice)')
    await expect(handle).not.toHaveAttribute('role')
    await expect(handle).not.toHaveAttribute('tabindex')

    await handle.click()
    await expect(page.locator('.floating-modal')).toHaveCount(0)
    await expect(page.locator('.session-context-menu')).toHaveCount(0)

    await row.locator('.session-name').click()
    await expect(page.locator('.floating-modal')).toBeVisible()
    await page.locator('.modal-close').click()

    await row.click({ button: 'right' })
    await expect(page.locator('.session-context-menu')).toBeVisible()
  })

  test('touchcancel before a handle drag leaves no stale menu timer, while ordinary row long-press still opens', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const handle = row.locator('.session-drag-handle')
    const name = row.locator('.session-name')
    const namePoint = await point(name)
    const cancelledTouch = { identifier: 7, clientX: namePoint.x, clientY: namePoint.y, screenX: namePoint.x, screenY: namePoint.y }

    await name.dispatchEvent('touchstart', { touches: [cancelledTouch], targetTouches: [cancelledTouch], changedTouches: [cancelledTouch] })
    await name.dispatchEvent('touchcancel', { touches: [], targetTouches: [], changedTouches: [cancelledTouch] })
    const pointerId = await startTouchPointerDrag(page, handle, page.locator('.tab-bar'))
    await page.waitForTimeout(600)
    await expect(page.locator('.session-context-menu')).toHaveCount(0)
    await cancelTouchPointerDrag(page, pointerId)

    const rowTouch = { identifier: 8, clientX: namePoint.x, clientY: namePoint.y, screenX: namePoint.x, screenY: namePoint.y }
    await name.dispatchEvent('touchstart', { touches: [rowTouch], targetTouches: [rowTouch], changedTouches: [rowTouch] })
    await page.waitForTimeout(600)
    await expect(page.locator('.session-context-menu')).toBeVisible()
    await name.dispatchEvent('touchend', { touches: [], targetTouches: [], changedTouches: [rowTouch] })
  })

  test('tag headers and same-window body are no-ops, cross-window body moves, and explicit removal paths detach', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const firstWindow = page.locator('.terminal-window:visible').nth(0)
    const secondWindow = page.locator('.terminal-window:visible').nth(1)
    const from = await point(row.locator('.session-name'))
    const to = await point(firstWindow.locator('.terminal-window-body'))

    await page.mouse.move(from.x, from.y)
    await page.mouse.down()
    await page.mouse.move(to.x, to.y, { steps: 10 })
    await page.mouse.up()
    await expectTagAssignment(firstWindow, secondWindow, 0, 0)

    await startMouseDrag(page, row.locator('.session-drag-handle'), firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    const tag = firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const tagHandle = tag.locator('.session-tag-drag-handle')

    await startMouseDrag(page, tagHandle, firstWindow.locator('.terminal-window-header'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await startMouseDrag(page, tagHandle, secondWindow.locator('.terminal-window-header'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await startMouseDrag(page, tagHandle, firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await startMouseDrag(page, tagHandle, secondWindow.locator('.terminal-window-body'))
    await expect(page.locator('.dragging-overlay')).toHaveCount(1)
    const overlay = page.locator('.dragging-overlay')
    await expect(overlay).toHaveClass(/session-tag/)
    await expect(overlay.locator('.drag-overlay-grip')).toHaveText('⠿')
    await expect(overlay.locator('.session-user-badge')).toHaveText('A')
    await expect(overlay.locator('.session-user-badge')).toHaveAttribute('title', 'Unix user: alice')
    await expect(tag).toHaveCSS('opacity', '0')
    await expect(tag).toHaveCSS('transform', 'none')
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 0, 1)

    const movedTag = secondWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const movedTagRemoveButton = movedTag.getByRole('button', { name: '×' })
    await expect(movedTagRemoveButton).toBeVisible()
    // dnd-kit retains a document-level click suppressor for 50ms after pointerup.
    // Keep this explicit remove click open past teardown so it reaches the moved tag.
    await movedTagRemoveButton.click({ delay: 75 })
    await expectTagAssignment(firstWindow, secondWindow, 0, 0)
    await expect(row).not.toHaveClass(/assigned/)

    await startMouseDrag(page, row.locator('.session-drag-handle'), firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await row.click({ button: 'right' })
    await page.getByRole('button', { name: /Unassign/i }).click()
    await expectTagAssignment(firstWindow, secondWindow, 0, 0)
    await expect(row).not.toHaveClass(/assigned/)
  })

  test('Escape explicitly restores iframe pointer events without moving the tag', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const firstWindow = page.locator('.terminal-window:visible').nth(0)
    const secondWindow = page.locator('.terminal-window:visible').nth(1)

    await startMouseDrag(page, row.locator('.session-drag-handle'), firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)

    const tag = firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const iframe = firstWindow.locator('iframe')
    await expect(iframe).toHaveCount(1)
    await startMouseDrag(page, tag.locator('.session-tag-drag-handle'), secondWindow.locator('.terminal-window-body'))
    await expect(iframe).toHaveCSS('pointer-events', 'none')

    await page.keyboard.press('Escape')
    await page.mouse.up()

    await expect(page.locator('.dragging-overlay')).toHaveCount(0)
    await expect(iframe).toHaveCSS('pointer-events', 'auto')
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)
  })

  test('narrow Chromium view renders no drag overlay inside display-none mobile frames', async ({ page }) => {
    await page.setViewportSize({ width: 700, height: 900 })
    await expect(page.getByText('View:').first()).toBeVisible()

    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
    const visibleWindow = windows.nth(0)
    const hiddenWindow = windows.nth(1)
    await expect(hiddenWindow).toHaveCSS('display', 'none')

    await startMouseDrag(page, row.locator('.session-drag-handle'), visibleWindow.locator('.terminal-window-body'))

    await expect(visibleWindow.locator('.terminal-drop-overlay')).toHaveCount(1)
    await expect(hiddenWindow.locator('.terminal-drop-overlay')).toHaveCount(0)
    await expect(page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-drop-overlay')).toHaveCount(1)
    await finishMouseDrag(page)
  })

  test('pointercancel clears overlay, drop feedback, source state, and iframe suppression', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const firstWindow = page.locator('.terminal-window:visible').nth(0)
    const secondWindow = page.locator('.terminal-window:visible').nth(1)

    await startMouseDrag(page, row.locator('.session-drag-handle'), firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)

    const tag = firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const handle = tag.locator('.session-tag-drag-handle')
    const iframe = firstWindow.locator('iframe')

    await startMouseDrag(page, handle, secondWindow.locator('.terminal-window-body'))
    await expect(iframe).toHaveCSS('pointer-events', 'none')
    await handle.dispatchEvent('pointercancel', {
      bubbles: true,
      pointerId: 1,
      pointerType: 'mouse',
      isPrimary: true,
    })
    await page.mouse.up()

    await expect(page.locator('.dashboard')).not.toHaveClass(/is-dragging/)
    await expect(page.locator('.dragging-overlay')).toHaveCount(0)
    await expect(page.locator('.terminal-drop-overlay')).toHaveCount(0)
    await expect(tag).not.toHaveClass(/dragging/)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)
    await expect(iframe).toHaveCSS('pointer-events', 'auto')
  })
})
