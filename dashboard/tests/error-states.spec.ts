import { test, expect, allowBrowserConsoleMessage, type Route } from './fixtures'
import { mockApiRoutes, mockFileApiRoutes, mockSessions } from './mock-api'
import { openSessionsSidecar } from './helpers'

/**
 * pol-0281: Error state E2E tests
 * Covers: API 500 on session creation, API timeout feedback,
 * double-click prevention, poll failure resilience, non-existent session binding.
 */

test.describe('Error States', () => {
  async function fulfillMockSessions(route: Route) {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockSessions),
    })
  }

  test.describe('API 500 on session creation', () => {
    test('should show error toast when POST /api/tmux/sessions returns 500', async ({ page }) => {
      allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 500')
      // Set up normal GET mock first, then override POST to fail
      await mockApiRoutes(page)
      await page.route('**/api/tmux/sessions', async route => {
        if (route.request().method() === 'POST') {
          await route.fulfill({
            status: 500,
            contentType: 'application/json',
            body: JSON.stringify({ error: 'tmux not running' }),
          })
        } else {
          await fulfillMockSessions(route)
        }
      })

      await page.goto('/')
      await page.waitForSelector('.dashboard')
      await openSessionsSidecar(page)
      await page.waitForSelector('.session-panel')

      // Find the "New Session" button in an empty window
      const createBtn = page.locator('.create-session-btn').first()
      await expect(createBtn).toBeVisible()

      // Click it
      await createBtn.click()

      // Error toast should appear
      const toast = page.locator('.toast-item.toast-error')
      await expect(toast).toBeVisible({ timeout: 5000 })
      await expect(toast.locator('.toast-message')).toContainText('Failed to create session')
    })
  })

  test.describe('API timeout shows loading feedback', () => {
    test('should show loading state on New Session button during request', async ({ page }) => {
      await mockApiRoutes(page)
      let postDone = false

      // Override POST to delay for a long time (simulating timeout)
      await page.route('**/api/tmux/sessions', async route => {
        if (route.request().method() === 'POST') {
          await new Promise(resolve => setTimeout(resolve, 750))
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({ name: 'shell-test' }),
          })
          postDone = true
        } else {
          await fulfillMockSessions(route)
        }
      })

      await page.goto('/')
      await page.waitForSelector('.dashboard')
      await openSessionsSidecar(page)

      const createBtn = page.locator('.create-session-btn').first()
      await expect(createBtn).toBeVisible()

      // Click to trigger session creation
      await createBtn.click()

      // Button should be disabled while creating
      await expect(createBtn).toBeDisabled()

      // The button icon should show "..." loading indicator
      await expect(createBtn.locator('.create-session-icon')).toHaveText('...')

      await expect(async () => {
        expect(postDone).toBe(true)
      }).toPass({ timeout: 2000 })
    })
  })

  test.describe('Double-click New Session prevention', () => {
    test('should disable button while creating, preventing duplicate sessions', async ({ page }) => {
      let postCount = 0
      let postDone = false
      await mockApiRoutes(page)

      // Track POST calls and add a delay so the button stays disabled
      await page.route('**/api/tmux/sessions', async route => {
        if (route.request().method() === 'POST') {
          postCount++
          // Delay the response so the button stays disabled
          await new Promise(resolve => setTimeout(resolve, 750))
          await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({ name: 'shell-test' }),
          })
          postDone = true
        } else {
          await fulfillMockSessions(route)
        }
      })

      await page.goto('/')
      await page.waitForSelector('.dashboard')
      await openSessionsSidecar(page)

      const createBtn = page.locator('.create-session-btn').first()
      await expect(createBtn).toBeVisible()

      // Click the button (first click)
      await createBtn.click()

      // Button should now be disabled
      await expect(createBtn).toBeDisabled()

      // Try clicking again — force: true bypasses Playwright's actionability check
      // to simulate what a rapid double-click would do
      await createBtn.click({ force: true }).catch(() => {
        // Expected: button is disabled, click may be rejected
      })

      // Verify only one POST was sent — use toPass to allow async settlement
      await expect(async () => {
        expect(postCount).toBe(1)
      }).toPass({ timeout: 3000 })

      await expect(async () => {
        expect(postDone).toBe(true)
      }).toPass({ timeout: 2000 })
    })
  })

  test.describe('Session poll failure does not clear existing layout', () => {
    test('should preserve bound sessions from localStorage when GET /api/tmux/sessions fails', async ({ page }) => {
      allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 500')
      // Pre-seed localStorage with a layout that has bound sessions
      const storedState = {
        workspaces: {
          terminal1: {
            windowCount: 2,
            windows: [
              {
                id: 'terminal1-window-0',
                boundSessions: ['hq-mayor'],
                activeSession: 'hq-mayor',
                colorIndex: 0,
              },
              {
                id: 'terminal1-window-1',
                boundSessions: ['main'],
                activeSession: 'main',
                colorIndex: 1,
              },
            ],
          },
          terminal2: {
            windowCount: 2,
            windows: [
              { id: 'terminal2-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
              { id: 'terminal2-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
            ],
          },
        },
        sidebarCollapsed: false,
        settings: {
          terminalMode: 'tmux',
          fontSize: 14,
          theme: 'matrix',
          autoRefreshInterval: 5000,
          defaultSessionPrefix: 'shell',
          musicVolume: 0.5,
          musicEnabled: false,
          tmuxAppearance: {
            statusBg: 'default',
            statusFg: '#00ff41',
            paneBorderActive: '#00ff41',
            paneBorderInactive: '#333333',
            modeStyleBg: '#00ff41',
            modeStyleFg: '#000000',
          },
        },
      }

      await page.route(/\/terminal(\/|\?|$)/, async route => {
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

      // Mock ALL GET requests to sessions to return 500 (server down)
      await page.route('**/api/tmux/sessions', async route => {
        if (route.request().method() === 'GET') {
          await route.fulfill({
            status: 500,
            contentType: 'application/json',
            body: JSON.stringify({ error: 'tmux server not running' }),
          })
        } else {
          await route.fulfill({ status: 500 })
        }
      })

      // Navigate and inject localStorage before the app loads
      await page.addInitScript((state) => {
        localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
      }, storedState)

      await page.goto('/')
      await page.waitForSelector('.dashboard')
      await openSessionsSidecar(page)

      // Wait for at least one poll cycle to complete (and fail) by checking
      // that the bound sessions from localStorage are rendered in the DOM
      await expect(page.locator('.terminal-window:visible .tag-name')).toHaveCount(2, { timeout: 5000 })

      // The bound sessions should still be visible as tags in the windows.
      // The SessionContext intentionally does NOT clean up orphaned sessions
      // (see comment in refreshSessions: "We intentionally do NOT clean up orphaned sessions")
      const window0 = page.locator('.terminal-window:visible').nth(0)
      const window1 = page.locator('.terminal-window:visible').nth(1)

      await expect(window0.locator('.tag-name')).toContainText('hq-mayor')
      await expect(window1.locator('.tag-name')).toContainText('main')
    })
  })

  test.describe('Binding a non-existent session', () => {
    test('should clear ghost session tag after successful API refresh confirms it is not live', async ({ page }) => {
      // Pre-seed localStorage with a session name that does not exist in the mock API data
      const storedState = {
        workspaces: {
          terminal1: {
            windowCount: 2,
            windows: [
              {
                id: 'terminal1-window-0',
                boundSessions: ['ghost-session-does-not-exist'],
                activeSession: 'ghost-session-does-not-exist',
                colorIndex: 0,
              },
              {
                id: 'terminal1-window-1',
                boundSessions: [],
                activeSession: null,
                colorIndex: 1,
              },
            ],
          },
          terminal2: {
            windowCount: 2,
            windows: [
              { id: 'terminal2-window-0', boundSessions: [], activeSession: null, colorIndex: 0 },
              { id: 'terminal2-window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
            ],
          },
        },
        sidebarCollapsed: false,
        settings: {
          terminalMode: 'tmux',
          fontSize: 14,
          theme: 'matrix',
          autoRefreshInterval: 5000,
          defaultSessionPrefix: 'shell',
          musicVolume: 0.5,
          musicEnabled: false,
          tmuxAppearance: {
            statusBg: 'default',
            statusFg: '#00ff41',
            paneBorderActive: '#00ff41',
            paneBorderInactive: '#333333',
            modeStyleBg: '#00ff41',
            modeStyleFg: '#000000',
          },
        },
      }

      await mockApiRoutes(page)

      await page.addInitScript((state) => {
        localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
      }, storedState)

      await page.goto('/')
      await page.waitForSelector('.dashboard')
      await openSessionsSidecar(page)

      // Wait for sessions to load from API
      await page.waitForSelector('.session-panel')

      // Repeated successful refreshes are authoritative. One miss is tolerated
      // to avoid dropping sessions during transient loading/user-list races.
      const window0 = page.locator('.terminal-window:visible').nth(0)
      await expect(window0.locator('.tag-name')).toHaveCount(0, { timeout: 12000 })
      await expect(window0.locator('button', { hasText: 'New Session' })).toBeVisible()

      // The ghost session should NOT appear in the sidebar session list
      // since it's not in the API response
      await expect(page.locator('.session-item:has-text("ghost-session")')).toHaveCount(0)

      // The stale binding cleanup should not mark unrelated live sessions assigned.
      const hqMayor = page.locator('.session-item:has-text("hq-mayor")')
      await expect(hqMayor).toBeVisible()
      await expect(hqMayor).not.toHaveClass(/assigned/)
    })
  })
})
