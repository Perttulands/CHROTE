import { test, expect } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'
import { openSessionsSidecar } from './helpers'

// Build a sessions payload that includes the protected 'chrote-chat' session
const sessionsWithProtected = {
  ...mockSessions,
  sessions: [
    ...mockSessions.sessions,
    { name: 'chrote-chat', windows: 1, attached: false, group: 'chrote' },
  ],
  grouped: {
    ...mockSessions.grouped,
    chrote: [
      { name: 'chrote-chat', windows: 1, attached: false, group: 'chrote' },
    ],
  },
}

test.describe('Nuke All (pol-9783)', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await openSessionsSidecar(page)
    await page.waitForSelector('.session-item')
    await page.getByRole('button', { name: 'Settings' }).click()
    await page.waitForSelector('.settings-view')
  })

  test('Nuke All button opens confirmation modal', async ({ page }) => {
    let deleteCalled = false
    await page.route('**/api/tmux/sessions/all', async route => {
      deleteCalled = true
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' })
    })
    await page.route('**/api/tmux/sessions', async route => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(sessionsWithProtected),
      })
    })
    await page.reload()
    await page.waitForSelector('.dashboard')
    await page.getByRole('button', { name: 'Settings' }).click()

    const nukeBtn = page.locator('.nuke-trigger-btn')
    await expect(nukeBtn).toBeVisible()
    await expect(nukeBtn).toContainText('Nuke All')
    await nukeBtn.click()
    await expect(page.locator('.nuke-modal')).toBeVisible()
    await expect(page.locator('.nuke-title')).toContainText('DESTROY ALL SESSIONS')

    const protectedText = page.locator('.nuke-protected')
    await expect(protectedText).toContainText('chrote-chat')
    await expect(protectedText).toContainText('preserved')

    const confirm = page.locator('.nuke-btn-confirm')
    await expect(confirm).toBeDisabled()
    await page.fill('.nuke-input', 'NUK')
    await expect(confirm).toBeDisabled()
    await page.fill('.nuke-input', 'NUKE')
    await expect(confirm).toBeEnabled()

    await page.locator('.nuke-btn-cancel').click()
    await expect(page.locator('.nuke-modal')).not.toBeVisible()
    expect(deleteCalled).toBe(false)
  })

  test('confirm sends DELETE request with correct header', async ({ page }) => {
    let deleteRequest: { method: string; headers: Record<string, string> } | null = null

    await page.route('**/api/tmux/sessions/all', async route => {
      const request = route.request()
      deleteRequest = {
        method: request.method(),
        headers: request.headers(),
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' })
    })

    // Open modal
    await page.locator('.nuke-trigger-btn').click()
    await expect(page.locator('.nuke-modal')).toBeVisible()

    // Type NUKE and confirm
    await page.fill('.nuke-input', 'NUKE')
    await page.locator('.nuke-btn-confirm').click()

    // Wait for the DELETE request to complete
    await expect(async () => {
      expect(deleteRequest).not.toBeNull()
    }).toPass({ timeout: 3000 })

    // Verify DELETE was sent
    expect(deleteRequest).not.toBeNull()
    expect(deleteRequest!.method).toBe('DELETE')
    expect(deleteRequest!.headers['x-nuke-confirm']).toBe('DASHBOARD-NUKE-CONFIRMED')
  })

})
