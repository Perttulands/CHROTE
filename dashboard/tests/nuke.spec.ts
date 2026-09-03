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

  test('the Nuke All button confirms in place and names what is preserved', async ({ page }) => {
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

    // The first press arms the same button and states the count and the
    // sessions the server will keep. Nothing opens over the work.
    await nukeBtn.click()
    await expect(nukeBtn).toContainText('Confirm: destroy')
    await expect(page.locator('.settings-danger-zone')).toContainText('chrote-chat')
    await expect(page.locator('.nuke-modal')).toHaveCount(0)
    expect(deleteCalled).toBe(false)
  })

  test('the second press sends DELETE with the confirmation header', async ({ page }) => {
    let deleteRequest: { method: string; headers: Record<string, string> } | null = null

    await page.route('**/api/tmux/sessions/all', async route => {
      const request = route.request()
      deleteRequest = {
        method: request.method(),
        headers: request.headers(),
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: '{}' })
    })

    const nukeBtn = page.locator('.nuke-trigger-btn')
    await nukeBtn.click()
    await expect(nukeBtn).toContainText('Confirm: destroy')
    await nukeBtn.click()

    await expect(async () => {
      expect(deleteRequest).not.toBeNull()
    }).toPass({ timeout: 3000 })

    expect(deleteRequest).not.toBeNull()
    expect(deleteRequest!.method).toBe('DELETE')
    expect(deleteRequest!.headers['x-nuke-confirm']).toBe('DASHBOARD-NUKE-CONFIRMED')
    await expect(page.locator('.status-line')).toContainText('All sessions destroyed')
  })

})
