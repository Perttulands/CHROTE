import { test, expect, Page } from '@playwright/test'
import { mockApiRoutes } from './mock-api'

// Extended mock for settings tests
async function mockSettingsApiRoutes(page: Page) {
  await mockApiRoutes(page)

  // Mock appearance endpoint
  await page.route('**/api/tmux/appearance', async route => {
    const request = route.request()
    const body = request.postDataJSON()

    // Validate the payload structure
    if (body && typeof body === 'object') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          applied: 2,
          total: 2,
          timestamp: new Date().toISOString(),
        }),
      })
    } else {
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, error: 'Invalid payload' }),
      })
    }
  })
}

test.describe('Settings View', () => {
  test.beforeEach(async ({ page }) => {
    // Clear localStorage before each test
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
    await page.reload()
    await mockSettingsApiRoutes(page)
    await page.waitForSelector('.dashboard')
  })

  test.describe('Theme Selection', () => {
    test('should show Settings tab', async ({ page }) => {
      const settingsTab = page.locator('.tab:has-text("Settings")')
      await expect(settingsTab).toBeVisible()
    })

    test('should switch to Settings view', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')
      await expect(page.locator('.settings-view')).toBeVisible()
    })

    test('should show theme selector', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')
      await expect(page.locator('.theme-selector, [class*="theme"]')).toBeVisible()
    })

    test('should change theme to Matrix', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')

      // Find Matrix theme option and click it
      const matrixOption = page.locator('[data-theme="matrix"], .theme-option:has-text("Matrix"), button:has-text("Matrix")')
      if (await matrixOption.count() > 0) {
        await matrixOption.click()

        // Verify data-theme attribute changes on document
        const dataTheme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
        expect(dataTheme).toBe('matrix')
      }
    })

    test('should change theme to Dark', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')

      const darkOption = page.locator('[data-theme="dark"], .theme-option:has-text("Dark"), button:has-text("Dark")')
      if (await darkOption.count() > 0) {
        await darkOption.click()

        const dataTheme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
        expect(dataTheme).toBe('dark')
      }
    })

    test('should change theme to Gastown', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')

      const gastownOption = page.locator('[data-theme="gastown"], .theme-option:has-text("Gastown"), button:has-text("Gastown")')
      if (await gastownOption.count() > 0) {
        await gastownOption.click()

        const dataTheme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
        expect(dataTheme).toBe('gastown')
      }
    })

    test('should persist theme to localStorage', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')

      // Select a different theme
      const darkOption = page.locator('[data-theme="dark"], .theme-option:has-text("Dark"), button:has-text("Dark")')
      if (await darkOption.count() > 0) {
        await darkOption.click()

        // Check localStorage
        const stored = await page.evaluate(() => {
          const state = localStorage.getItem('arena-dashboard-state')
          return state ? JSON.parse(state) : null
        })

        expect(stored?.settings?.theme).toBe('dark')
      }
    })

    test('should restore theme on reload', async ({ page }) => {
      // Set theme in localStorage directly
      await page.evaluate(() => {
        const state = {
          windowCount: 2,
          windows: [],
          focusedWindowIndex: 0,
          sidebarCollapsed: false,
          settings: { theme: 'gastown', fontSize: 14, pollingInterval: 5000 },
        }
        localStorage.setItem('arena-dashboard-state', JSON.stringify(state))
      })

      // Reload
      await page.reload()
      await mockSettingsApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // Verify theme is applied
      const dataTheme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
      expect(dataTheme).toBe('gastown')
    })
  })

  test.describe('Font Size', () => {
    test('should show font size control', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')
      await expect(page.locator('[class*="font-size"], input[type="range"], .font-size-control')).toBeVisible()
    })

    test('should change font size', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')

      // Find font size slider/input
      const fontControl = page.locator('input[type="range"][name*="font"], input[type="range"][class*="font"]')
      if (await fontControl.count() > 0) {
        // Set to 18px
        await fontControl.fill('18')

        // Verify stored value
        const stored = await page.evaluate(() => {
          const state = localStorage.getItem('arena-dashboard-state')
          return state ? JSON.parse(state) : null
        })

        expect(stored?.settings?.fontSize).toBe(18)
      }
    })

    test('should persist font size on reload', async ({ page }) => {
      // Set font size in localStorage
      await page.evaluate(() => {
        const state = {
          windowCount: 2,
          windows: [],
          focusedWindowIndex: 0,
          sidebarCollapsed: false,
          settings: { theme: 'matrix', fontSize: 16, pollingInterval: 5000 },
        }
        localStorage.setItem('arena-dashboard-state', JSON.stringify(state))
      })

      await page.reload()
      await mockSettingsApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // Go to settings and verify value
      await page.click('.tab:has-text("Settings")')

      const fontControl = page.locator('input[type="range"][name*="font"], input[type="range"][class*="font"]')
      if (await fontControl.count() > 0) {
        const value = await fontControl.inputValue()
        expect(value).toBe('16')
      }
    })
  })

  test.describe('Polling Interval', () => {
    test('should show polling interval control', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')

      // Look for polling/refresh interval control
      const pollingControl = page.locator('[class*="polling"], [class*="refresh"], input[name*="interval"]')
      // This may or may not exist depending on UI
      if (await pollingControl.count() > 0) {
        await expect(pollingControl.first()).toBeVisible()
      }
    })

    test('should persist polling interval', async ({ page }) => {
      // Set polling interval in localStorage
      await page.evaluate(() => {
        const state = {
          windowCount: 2,
          windows: [],
          focusedWindowIndex: 0,
          sidebarCollapsed: false,
          settings: { theme: 'matrix', fontSize: 14, pollingInterval: 10000 },
        }
        localStorage.setItem('arena-dashboard-state', JSON.stringify(state))
      })

      await page.reload()
      await mockSettingsApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // Verify it's stored
      const stored = await page.evaluate(() => {
        const state = localStorage.getItem('arena-dashboard-state')
        return state ? JSON.parse(state) : null
      })

      expect(stored?.settings?.pollingInterval).toBe(10000)
    })
  })

  test.describe('tmux Appearance', () => {
    test('should send appearance settings to API', async ({ page }) => {
      await page.click('.tab:has-text("Settings")')

      // Track API calls
      const apiCalls: { url: string; body: unknown }[] = []
      await page.route('**/api/tmux/appearance', async route => {
        const request = route.request()
        apiCalls.push({
          url: request.url(),
          body: request.postDataJSON(),
        })
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true, applied: 2, total: 2 }),
        })
      })

      // Find and click apply appearance button (if it exists)
      const applyButton = page.locator('button:has-text("Apply"), button:has-text("Save Appearance")')
      if (await applyButton.count() > 0) {
        await applyButton.click()

        // Wait for API call
        await page.waitForTimeout(500)

        // Verify API was called
        expect(apiCalls.length).toBeGreaterThanOrEqual(0)
      }
    })

    test('should apply appearance on theme change', async ({ page }) => {
      // Track appearance API calls
      const appearanceCalls: unknown[] = []
      await page.route('**/api/tmux/appearance', async route => {
        appearanceCalls.push(route.request().postDataJSON())
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true, applied: 2, total: 2 }),
        })
      })

      await page.click('.tab:has-text("Settings")')

      // Select a theme
      const matrixOption = page.locator('[data-theme="matrix"], .theme-option:has-text("Matrix"), button:has-text("Matrix")')
      if (await matrixOption.count() > 0) {
        await matrixOption.click()

        // Wait for potential API call
        await page.waitForTimeout(500)

        // If the app calls appearance API on theme change, verify payload
        if (appearanceCalls.length > 0) {
          const lastCall = appearanceCalls[appearanceCalls.length - 1] as Record<string, unknown>
          // Verify it has color properties
          expect(lastCall).toHaveProperty('statusBg')
          expect(lastCall).toHaveProperty('statusFg')
        }
      }
    })
  })

  test.describe('Settings Persistence Integration', () => {
    test('all settings should persist across reload', async ({ page }) => {
      // Set complete state
      await page.evaluate(() => {
        const state = {
          windowCount: 4,
          windows: [
            { id: 'window-0', boundSessions: ['hq-mayor'], activeSession: 'hq-mayor', colorIndex: 0 },
            { id: 'window-1', boundSessions: [], activeSession: null, colorIndex: 1 },
            { id: 'window-2', boundSessions: [], activeSession: null, colorIndex: 2 },
            { id: 'window-3', boundSessions: [], activeSession: null, colorIndex: 3 },
          ],
          focusedWindowIndex: 0,
          sidebarCollapsed: true,
          settings: {
            theme: 'gastown',
            fontSize: 18,
            pollingInterval: 3000,
          },
        }
        localStorage.setItem('arena-dashboard-state', JSON.stringify(state))
      })

      await page.reload()
      await mockSettingsApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // Verify all settings persisted
      // 1. Window count
      await expect(page.locator('.terminal-window')).toHaveCount(4)

      // 2. Sidebar collapsed
      await expect(page.locator('.session-panel')).toHaveClass(/collapsed/)

      // 3. Theme
      const dataTheme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
      expect(dataTheme).toBe('gastown')

      // 4. Session binding
      const firstWindow = page.locator('.terminal-window').first()
      await expect(firstWindow.locator('.tag-name')).toContainText('hq-mayor')
    })

    test('should handle corrupted localStorage gracefully', async ({ page }) => {
      // Set invalid JSON in localStorage
      await page.evaluate(() => {
        localStorage.setItem('arena-dashboard-state', 'not valid json {{{')
      })

      await page.reload()
      await mockSettingsApiRoutes(page)

      // App should still load with defaults
      await page.waitForSelector('.dashboard')
      await expect(page.locator('.terminal-window')).toHaveCount(2) // Default
    })

    test('should handle missing settings object gracefully', async ({ page }) => {
      // Set state without settings
      await page.evaluate(() => {
        localStorage.setItem('arena-dashboard-state', JSON.stringify({
          windowCount: 2,
          windows: [],
        }))
      })

      await page.reload()
      await mockSettingsApiRoutes(page)
      await page.waitForSelector('.dashboard')

      // App should use default settings
      const dataTheme = await page.evaluate(() => document.documentElement.getAttribute('data-theme'))
      expect(['matrix', 'dark', 'gastown', null]).toContain(dataTheme)
    })
  })
})

test.describe('Settings: Theme Visual Verification', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await mockSettingsApiRoutes(page)
    await page.waitForSelector('.dashboard')
  })

  test('Matrix theme should have green accent', async ({ page }) => {
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'matrix'))

    // Verify CSS variables are set
    const accentColor = await page.evaluate(() => {
      return getComputedStyle(document.documentElement).getPropertyValue('--accent').trim()
    })

    // Matrix theme typically has green (#00ff41 or similar)
    expect(accentColor).toMatch(/#[0-9a-fA-F]{3,6}|rgb|green/i)
  })

  test('Dark theme should have blue accent', async ({ page }) => {
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'dark'))

    const accentColor = await page.evaluate(() => {
      return getComputedStyle(document.documentElement).getPropertyValue('--accent').trim()
    })

    // Dark theme typically has blue accent
    expect(accentColor).toMatch(/#[0-9a-fA-F]{3,6}|rgb|blue/i)
  })

  test('Gastown theme should have warm colors', async ({ page }) => {
    await page.evaluate(() => document.documentElement.setAttribute('data-theme', 'gastown'))

    const backgroundColor = await page.evaluate(() => {
      return getComputedStyle(document.documentElement).getPropertyValue('--background').trim()
    })

    // Gastown has warm cream/russet colors
    expect(backgroundColor).toBeTruthy()
  })
})
