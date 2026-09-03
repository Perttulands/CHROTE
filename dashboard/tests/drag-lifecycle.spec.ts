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

  test('row names assign; tag headers and same-window body are no-ops; removal detaches; Escape abandons', async ({ page }) => {
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
    await expect(firstWindow.getByRole('button', { name: 'Send to session gt-gastown-jack' })).toBeVisible()

    const tag = firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const tagDragSurface = tag

    await startMouseDrag(page, tagDragSurface, firstWindow.locator('.terminal-window-header'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await startMouseDrag(page, tagDragSurface, secondWindow.locator('.terminal-window-header'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    await startMouseDrag(page, tagDragSurface, firstWindow.locator('.terminal-window-body'))
    // Hovering the tag's own source window is a no-op, so it shows no drop feedback.
    await expect(page.locator('.terminal-drop-overlay')).toHaveCount(0)
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
    await page.getByRole('menuitem', { name: /Unassign/i }).click()
    await expectTagAssignment(firstWindow, secondWindow, 0, 0)
    await expect(row).not.toHaveClass(/assigned/)

    // Escape abandons a drag in flight. The tag stays where it was and, more
    // to the point, the terminal underneath gets its pointer events back: if
    // that lift stuck, every terminal would go dead to the mouse.
    await startMouseDrag(page, row, firstWindow.locator('.terminal-window-body'))
    await finishMouseDrag(page)
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)

    const assignedTag = firstWindow.locator('.session-tag:has-text("gt-gastown-jack")')
    const terminal = firstWindow.locator('.terminal-surface-host')
    await expect(terminal).toHaveCount(1)
    await terminal.evaluate(element => { element.setAttribute('data-drag-identity', 'preserved') })

    await startMouseDrag(page, assignedTag, secondWindow.locator('.terminal-window-body'))
    await expect(terminal).toHaveCSS('pointer-events', 'none')

    await page.keyboard.press('Escape')
    await page.mouse.up()

    await expect(page.locator('.dragging-overlay')).toHaveCount(0)
    await expect(terminal).toHaveCSS('pointer-events', 'auto')
    await expect(terminal).toHaveAttribute('data-drag-identity', 'preserved')
    await expectTagAssignment(firstWindow, secondWindow, 1, 0)
  })

  // The seam between two tiles is the layout's own drop target: it appears only
  // while a session is in the air, and dropping there makes the window it needs.
  test('a drop in the seam between tiles adds a window and lands the session in it', async ({ page }) => {
    const row = page.locator('.session-panel .session-item:has-text("gt-gastown-jack")')
    const windows = page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
    await expect(windows).toHaveCount(2)
    await expect(page.locator('.terminal-window-gap')).toHaveCount(0)

    const from = await point(row)
    const firstBox = await windows.first().boundingBox()
    const secondBox = await windows.nth(1).boundingBox()
    if (!firstBox || !secondBox) throw new Error('the tiles have no bounding box')
    const seam = {
      x: (firstBox.x + firstBox.width + secondBox.x) / 2,
      y: firstBox.y + firstBox.height / 2,
    }

    await page.mouse.move(from.x, from.y)
    await page.mouse.down()
    await page.mouse.move(from.x + 12, from.y + 12, { steps: 4 })
    await page.mouse.move(seam.x, seam.y, { steps: 8 })
    await expect(page.locator('.terminal-window-gap')).toHaveCount(1)
    await expect(page.locator('.terminal-window-gap.over')).toHaveCount(1)

    await page.mouse.up()
    await expect(page.locator('.terminal-window-gap')).toHaveCount(0)
    await expect(windows).toHaveCount(3)
    await expect(windows.nth(2).locator('.session-tag')).toContainText('gt-gastown-jack')
    await expect(windows.first().locator('.session-tag')).toHaveCount(0)
  })

})
