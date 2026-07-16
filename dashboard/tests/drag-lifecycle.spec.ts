import { expect, test, type Locator, type Page } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'
import { openSessionsSidecar } from './helpers'

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
    await openSessionsSidecar(page)
    await page.waitForSelector('.session-panel .session-item')
  })

  test('only the overlay moves mid-drag and drop feedback has an opaque base', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const target = page.locator('.terminal-window:visible').first().locator('.terminal-window-body')
    const initialBox = await row.boundingBox()

    await expect(row).toHaveCSS('touch-action', 'pan-y')
    await startMouseDrag(page, row, target)

    await expect(row).toHaveClass(/dragging/)
    await expect(row).toHaveCSS('opacity', '0')
    await expect(row).toHaveCSS('transform', 'none')
    await expect(row).toHaveCSS('transition-property', 'none')
    const dragBox = await row.boundingBox()
    expect(dragBox).toBeTruthy()
    expect(dragBox!.width).toBe(initialBox!.width)
    expect(dragBox!.height).toBe(initialBox!.height)
    expect(dragBox!.x).toBe(initialBox!.x)
    expect(Math.abs(dragBox!.y - initialBox!.y)).toBeLessThanOrEqual(4)

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

  test('sub-8px movement stays inactive and a real touch pointer drag activates from the whole row', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const from = await point(row)

    await page.mouse.move(from.x, from.y)
    await page.mouse.down()
    await page.mouse.move(from.x + 7, from.y)
    await expect(page.locator('.dashboard')).not.toHaveClass(/is-dragging/)
    await expect(page.locator('.dragging-overlay')).toHaveCount(0)
    await page.mouse.up()

    const pointerId = await startTouchPointerDrag(page, row, page.locator('.tab-bar'))
    await expect(row).toHaveClass(/dragging/)
    await expect(page.locator('.dragging-overlay')).toHaveCount(1)
    await cancelTouchPointerDrag(page, pointerId)
    await expect(row).not.toHaveClass(/dragging/)
  })

  test('renders no grip while nested controls, row click, and right-click remain available', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')

    await expect(row.locator('.session-drag-handle')).toHaveCount(0)
    await expect(row).toHaveAttribute('title', 'Drag gt-gastown-jack (Unix user alice)')
    const actions = row.getByRole('button', { name: 'Session actions for gt-gastown-jack' })
    await actions.click()
    await expect(page.locator('.session-context-menu')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.locator('.session-context-menu')).toHaveCount(0)

    await row.locator('.session-name').click()
    await expect(page.locator('.floating-modal')).toBeVisible()
    await page.locator('.modal-close').click()

    await row.click({ button: 'right' })
    await expect(page.locator('.session-context-menu')).toBeVisible()
  })

  test('touchcancel before a row drag leaves no stale menu timer, while ordinary row long-press still opens', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const name = row.locator('.session-name')
    const namePoint = await point(name)
    const cancelledTouch = { identifier: 7, clientX: namePoint.x, clientY: namePoint.y, screenX: namePoint.x, screenY: namePoint.y }

    await name.dispatchEvent('touchstart', { touches: [cancelledTouch], targetTouches: [cancelledTouch], changedTouches: [cancelledTouch] })
    await name.dispatchEvent('touchcancel', { touches: [], targetTouches: [], changedTouches: [cancelledTouch] })
    const pointerId = await startTouchPointerDrag(page, row, page.locator('.tab-bar'))
    await page.waitForTimeout(600)
    await expect(page.locator('.session-context-menu')).toHaveCount(0)
    await cancelTouchPointerDrag(page, pointerId)

    const rowTouch = { identifier: 8, clientX: namePoint.x, clientY: namePoint.y, screenX: namePoint.x, screenY: namePoint.y }
    await name.dispatchEvent('touchstart', { touches: [rowTouch], targetTouches: [rowTouch], changedTouches: [rowTouch] })
    await page.waitForTimeout(600)
    await expect(page.locator('.session-context-menu')).toBeVisible()
    await name.dispatchEvent('touchend', { touches: [], targetTouches: [], changedTouches: [rowTouch] })
  })

  test('a trusted long-press opens actions and cancels drag activation for that touch', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const from = await point(row.locator('.session-name'))
    const cdp = await page.context().newCDPSession(page)
    await cdp.send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 1 })

    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x: from.x, y: from.y, id: 51, radiusX: 1, radiusY: 1, force: 1 }],
    })
    await page.waitForTimeout(600)
    await expect(page.locator('.session-context-menu')).toBeVisible()

    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: from.x + 20, y: from.y + 4, id: 51, radiusX: 1, radiusY: 1, force: 1 }],
    })
    await expect(page.locator('.dashboard')).not.toHaveClass(/is-dragging/)
    await expect(page.locator('.dragging-overlay')).toHaveCount(0)
    await expect(page.locator('.session-context-menu')).toBeVisible()

    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await cdp.detach()
  })

  test('row names assign; tag headers and same-window body are no-ops; explicit removal paths detach', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const firstWindow = page.locator('.terminal-window:visible').nth(0)
    const secondWindow = page.locator('.terminal-window:visible').nth(1)
    const from = await point(row.locator('.session-name'))
    const to = await point(firstWindow.locator('.terminal-window-body'))

    await page.mouse.move(from.x, from.y)
    await page.mouse.down()
    await page.mouse.move(to.x, to.y, { steps: 10 })
    await page.mouse.up()
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    const tag = firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const tagDragSurface = tag

    await startMouseDrag(page, tagDragSurface, firstWindow.locator('.terminal-window-header'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await startMouseDrag(page, tagDragSurface, secondWindow.locator('.terminal-window-header'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await startMouseDrag(page, tagDragSurface, firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await startMouseDrag(page, tagDragSurface, secondWindow.locator('.terminal-window-body'))
    await expect(page.locator('.dragging-overlay')).toHaveCount(1)
    const overlay = page.locator('.dragging-overlay')
    await expect(overlay).toHaveClass(/session-tag/)
    await expect(overlay.locator('.drag-overlay-grip')).toHaveCount(0)
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

    await startMouseDrag(page, row, firstWindow.locator('.terminal-window-body'))
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

    await startMouseDrag(page, row, firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)

    const tag = firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const iframe = firstWindow.locator('iframe')
    await expect(iframe).toHaveCount(1)
    await iframe.evaluate(element => {
      element.setAttribute('data-drag-identity', 'preserved')
      element.setAttribute('data-drag-loads', '0')
      element.addEventListener('load', () => {
        const loads = Number(element.getAttribute('data-drag-loads') || '0') + 1
        element.setAttribute('data-drag-loads', String(loads))
      })
    })
    await startMouseDrag(page, tag, secondWindow.locator('.terminal-window-body'))
    await expect(iframe).toHaveCSS('pointer-events', 'none')

    await page.keyboard.press('Escape')
    await page.mouse.up()

    await expect(page.locator('.dragging-overlay')).toHaveCount(0)
    await expect(iframe).toHaveCSS('pointer-events', 'auto')
    await expect(iframe).toHaveAttribute('data-drag-identity', 'preserved')
    await expect(iframe).toHaveAttribute('data-drag-loads', '0')
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)
  })

  test('narrow Chromium view renders no drag overlay inside display-none mobile frames', async ({ page }) => {
    await page.setViewportSize({ width: 700, height: 900 })
    await expect(page.getByText('View:').first()).toBeVisible()
    await expect(page.locator('.session-panel')).toHaveClass(/sidecar-overlay/)
    await expect(page.getByRole('button', { name: 'Pin Sessions sidecar' })).toHaveCount(0)
    await expect(page.locator('.session-panel .session-item').first()).toBeVisible()
    await page.waitForTimeout(250)

    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
    const visibleWindow = windows.nth(0)
    const hiddenWindow = windows.nth(1)
    await expect(hiddenWindow).toHaveCSS('display', 'none')

    await startMouseDrag(page, row, visibleWindow.locator('.terminal-window-body'))

    await expect(visibleWindow.locator('.terminal-drop-overlay')).toHaveCount(1)
    await expect(hiddenWindow.locator('.terminal-drop-overlay')).toHaveCount(0)
    await expect(page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-drop-overlay')).toHaveCount(1)
    await finishMouseDrag(page)
  })

  test('pointercancel clears overlay, drop feedback, source state, and iframe suppression', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const firstWindow = page.locator('.terminal-window:visible').nth(0)
    const secondWindow = page.locator('.terminal-window:visible').nth(1)

    await startMouseDrag(page, row, firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)

    const tag = firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const handle = tag
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
