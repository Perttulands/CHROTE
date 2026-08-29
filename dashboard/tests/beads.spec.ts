import { test, expect, allowBrowserConsoleMessage } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockBeadsApiError } from './mock-api'

test.describe('Beads View', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test.describe('Navigation', () => {

    test('should open with the first discovered project selected', async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')

      await expect(page.locator('#project-select')).toHaveValue('/code/test-project')
      await expect(page.locator('.beads-subtabs')).toBeVisible()
      await expect(page.locator('.beads-kanban')).toBeVisible()
    })

  })

  test.describe('Sub-tab Navigation', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      // Select a project
      await page.selectOption('#project-select', '/code/test-project')
      // Wait for content to load
      await page.waitForSelector('.beads-subtabs')
    })

    test('should switch to Kanban view when clicking Kanban tab', async ({ page }) => {
      // First click another tab
      await page.click('.beads-subtab:has-text("Triage")')
      await expect(page.locator('.beads-triage')).toBeVisible()

      // Then click Kanban
      await page.click('.beads-subtab:has-text("Kanban")')
      await expect(page.locator('.beads-subtab:has-text("Kanban")')).toHaveClass(/active/)
      await expect(page.locator('.beads-kanban')).toBeVisible()
    })

  })

  test.describe('Error Handling', () => {
    test('should display error message when API fails', async ({ page }) => {
      allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 503')
      // Override with error mocks
      await mockBeadsApiError(page)

      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      await page.selectOption('#project-select', '/code/test-project')

      // Should show error state, not blank screen
      await expect(page.locator('.beads-view')).toBeVisible()
      // Error message should be visible
      await expect(page.locator('.error-message')).toBeVisible()
    })

  })

  test.describe('Refresh Button', () => {

    test('should refresh data when clicking refresh', async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      await page.selectOption('#project-select', '/code/test-project')

      await page.click('.beads-refresh-btn')

      // Should still have content after refresh
      await expect(page.locator('.beads-view')).toBeVisible()
    })
  })
})
