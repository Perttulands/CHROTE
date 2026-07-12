import { test, expect, Page } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'

const terminalRoutePattern = /.*\/terminal\/?.*/

// Helper: drag-and-drop for dnd-kit (same as dashboard.spec.ts)
async function dragAndDrop(page: Page, sourceSelector: string, targetSelector: string) {
  const source = page.locator(sourceSelector).first().locator('.session-drag-handle')
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
  await page.waitForTimeout(100) // drag settle — no event to wait for during mouse drag
  await page.mouse.up()
  await page.waitForTimeout(100) // drop settle — browser needs time to process mouseup + dragend
}

// Build a mock response with the renamed session
function buildRenamedSessions(oldName: string, newName: string) {
  const sessions = mockSessions.sessions.map(s =>
    s.name === oldName ? { ...s, name: newName } : s
  )
  const grouped: Record<string, typeof sessions> = {}
  for (const [key, groupSessions] of Object.entries(mockSessions.grouped)) {
    grouped[key] = (groupSessions as typeof sessions).map(s =>
      s.name === oldName ? { ...s, name: newName } : s
    )
  }
  return { ...mockSessions, sessions, grouped }
}

test.describe('Rename Propagation (pol-ace3)', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    // Mock terminal iframe requests
    await page.route(terminalRoutePattern, async route => {
      await route.fulfill({
        status: 200,
        contentType: 'text/html',
        body: '<html><body>mock terminal</body></html>',
      })
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.waitForSelector('.session-item')
  })

  test('bind session to window, rename via context menu, verify tag updates', async ({ page }) => {
    const oldName = 'gt-gastown-jack'
    const newName = 'renamed-jack'

    // 1. Bind "jack" session to the first terminal window
    await dragAndDrop(page, `.session-item:has-text("${oldName}")`, '.terminal-window')

    // Verify the tag shows the original name
    const targetWindow = page.locator('.terminal-window').first()
    await expect(targetWindow.locator('.tag-name')).toContainText(oldName)

    // 2. Mock the PATCH rename endpoint
    let renameRequestBody: { newName: string } | null = null
    await page.route(`**/api/tmux/sessions/${encodeURIComponent(oldName)}`, async route => {
      if (route.request().method() === 'PATCH') {
        renameRequestBody = JSON.parse(route.request().postData() || '{}')
        await route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' })
      } else {
        await route.fallback()
      }
    })

    // After rename, the sessions API should return the new name
    const renamedSessions = buildRenamedSessions(oldName, newName)
    // We need to override the existing route. Use unroute + re-route.
    await page.unroute('**/api/tmux/sessions')
    // This route will serve the renamed data on the next poll
    let renameCompleted = false
    await page.route('**/api/tmux/sessions', async route => {
      if (renameCompleted) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(renamedSessions),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(mockSessions),
        })
      }
    })

    // 3. Right-click on the session to open context menu
    const sessionItem = page.locator(`.session-panel .session-item:has-text("${oldName}")`)
    await sessionItem.click({ button: 'right' })

    // Context menu should appear
    await expect(page.locator('.session-context-menu')).toBeVisible()

    // Click "Rename"
    await page.locator('.session-context-item:has-text("Rename")').click()

    // Rename input should appear
    const renameInput = page.locator('.session-rename-input')
    await expect(renameInput).toBeVisible()

    // Clear and type new name
    await renameInput.fill(newName)

    // Mark rename as completed so next refresh returns new data
    renameCompleted = true

    // Press Enter to confirm
    await renameInput.press('Enter')

    // Wait for the rename API call to be captured
    await expect(async () => {
      expect(renameRequestBody).not.toBeNull()
    }).toPass({ timeout: 3000 })
    expect(renameRequestBody!.newName).toBe(newName)

    // 4. Verify the session tag in the terminal window updates to the new name
    await expect(targetWindow.locator('.tag-name')).toContainText(newName)

    // 5. Verify the session panel also shows the new name
    await expect(page.locator(`.session-item:has-text("${newName}")`)).toBeVisible()
  })

  test('renamed binding persists in localStorage after reload', async ({ page }) => {
    const oldName = 'gt-gastown-jack'
    const newName = 'renamed-jack'

    // 1. Bind session to window
    await dragAndDrop(page, `.session-item:has-text("${oldName}")`, '.terminal-window')
    await expect(page.locator('.terminal-window').first().locator('.tag-name')).toContainText(oldName)

    // 2. Mock rename endpoint
    await page.route(`**/api/tmux/sessions/${encodeURIComponent(oldName)}`, async route => {
      if (route.request().method() === 'PATCH') {
        await route.fulfill({ status: 200, contentType: 'application/json', body: '{"ok":true}' })
      } else {
        await route.fallback()
      }
    })

    // 3. Rename via context menu
    const sessionItem = page.locator(`.session-panel .session-item:has-text("${oldName}")`)
    await sessionItem.click({ button: 'right' })
    await page.locator('.session-context-item:has-text("Rename")').click()
    const renameInput = page.locator('.session-rename-input')
    await renameInput.fill(newName)
    await renameInput.press('Enter')

    // Wait for state to persist to localStorage
    await expect(async () => {
      const raw = await page.evaluate(() => localStorage.getItem('chrote-dashboard-state'))
      expect(raw).not.toBeNull()
    }).toPass({ timeout: 3000 })

    // 4. Verify localStorage has the new name in workspaces
    await expect(async () => {
      const allBoundSessions = await page.evaluate(() => {
        const raw = localStorage.getItem('chrote-dashboard-state')
        const storedState = raw ? JSON.parse(raw) : null
        const workspaces = storedState?.layoutsByViewport?.desktop?.workspaces ?? storedState?.workspaces
        if (!workspaces) return []

        const sessions: string[] = []
        for (const wsKey of ['terminal1', 'terminal2']) {
          const ws = workspaces[wsKey]
          for (const w of ws.windows) {
            sessions.push(...w.boundSessions)
          }
        }
        return sessions
      })

      expect(allBoundSessions).toContain(newName)
      expect(allBoundSessions).not.toContain(oldName)
    }).toPass({ timeout: 3000 })

    // 5. Reload page with the renamed session data
    const renamedSessions = buildRenamedSessions(oldName, newName)
    await page.route('**/api/tmux/sessions', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(renamedSessions),
      })
    })

    await page.reload()
    await page.waitForSelector('.dashboard')

    // The tag should still show the renamed session after reload
    await expect(page.locator('.terminal-window').first().locator('.tag-name')).toContainText(newName)
  })
})
