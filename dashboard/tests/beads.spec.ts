import { test, expect } from '@playwright/test'
import { mockApiRoutes, mockBeadsApiRoutes, mockBeadsApiError } from './mock-api'

test.describe('Beads View', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test.describe('Navigation', () => {
    test('should switch to Beads tab', async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await expect(page.locator('.beads-view')).toBeVisible()
    })

    test('should render BeadsView with project selector', async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await expect(page.locator('.project-selector')).toBeVisible()
      await expect(page.locator('#project-select')).toBeVisible()
    })

    test('should show empty state when no project selected', async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await expect(page.locator('.beads-empty-state')).toBeVisible()
      await expect(page.locator('.beads-empty-state h2')).toContainText('Select a Project')
    })

    test('should load and display projects in dropdown', async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')

      // Check that projects are loaded
      const options = page.locator('#project-select option')
      await expect(options).toHaveCount(3) // "Select a project" + 2 mock projects
      await expect(options.nth(1)).toContainText('test-project')
      await expect(options.nth(2)).toContainText('another-project')
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

    test('should display sub-tabs after selecting project', async ({ page }) => {
      await expect(page.locator('.beads-subtab')).toHaveCount(3)
      await expect(page.locator('.beads-subtab:has-text("Kanban")')).toBeVisible()
      await expect(page.locator('.beads-subtab:has-text("Triage")')).toBeVisible()
      await expect(page.locator('.beads-subtab:has-text("Insights")')).toBeVisible()
    })

    test('should show Kanban view by default', async ({ page }) => {
      await expect(page.locator('.beads-subtab:has-text("Kanban")')).toHaveClass(/active/)
      await expect(page.locator('.beads-kanban')).toBeVisible()
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

    test('should switch to Triage view when clicking Triage tab', async ({ page }) => {
      await page.click('.beads-subtab:has-text("Triage")')
      await expect(page.locator('.beads-subtab:has-text("Triage")')).toHaveClass(/active/)
      await expect(page.locator('.beads-triage')).toBeVisible()
    })

    test('should switch to Insights view when clicking Insights tab', async ({ page }) => {
      await page.click('.beads-subtab:has-text("Insights")')
      await expect(page.locator('.beads-subtab:has-text("Insights")')).toHaveClass(/active/)
      await expect(page.locator('.beads-insights')).toBeVisible()
    })

    test('should not crash when rapidly switching tabs', async ({ page }) => {
      // Rapidly switch between tabs
      await page.click('.beads-subtab:has-text("Triage")')
      await page.click('.beads-subtab:has-text("Insights")')
      await page.click('.beads-subtab:has-text("Kanban")')
      await page.click('.beads-subtab:has-text("Triage")')
      await page.click('.beads-subtab:has-text("Insights")')

      // Should still have content
      await expect(page.locator('.beads-view')).toBeVisible()
      await expect(page.locator('.beads-content')).toBeVisible()
    })
  })

  test.describe('Kanban View Content', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      await page.selectOption('#project-select', '/code/test-project')
      await page.waitForSelector('.beads-kanban')
    })

    test('should display kanban columns', async ({ page }) => {
      await expect(page.locator('.kanban-column')).toHaveCount(5) // open, ready, in_progress, blocked, closed
    })

    test('should display issues in columns', async ({ page }) => {
      // Wait for issues to load
      await page.waitForSelector('.issue-card')
      await expect(page.locator('.issue-card')).toHaveCount(5)
    })

    test('should show issue details', async ({ page }) => {
      await page.waitForSelector('.issue-card')
      await expect(page.locator('.issue-id:has-text("ISSUE-001")')).toBeVisible()
      await expect(page.locator('.issue-card-title:has-text("Fix login bug")')).toBeVisible()
    })
  })

  test.describe('Triage View Content', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      await page.selectOption('#project-select', '/code/test-project')
      await page.click('.beads-subtab:has-text("Triage")')
      await page.waitForSelector('.beads-triage')
    })

    test('should display triage sections', async ({ page }) => {
      await expect(page.locator('.triage-section.recommendations')).toBeVisible()
      await expect(page.locator('.triage-section.quick-wins')).toBeVisible()
      await expect(page.locator('.triage-section.blockers')).toBeVisible()
    })

    test('should display recommendations', async ({ page }) => {
      await expect(page.locator('.recommendation-item')).toHaveCount(2)
      await expect(page.locator('.rec-rank:has-text("#1")')).toBeVisible()
    })
  })

  test.describe('Insights View Content', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      await page.selectOption('#project-select', '/code/test-project')
      await page.click('.beads-subtab:has-text("Insights")')
      await page.waitForSelector('.beads-insights')
    })

    test('should display health gauge', async ({ page }) => {
      await expect(page.locator('.health-gauge')).toBeVisible()
      await expect(page.locator('.gauge-value')).toContainText('75')
    })

    test('should display stats section', async ({ page }) => {
      await expect(page.locator('.stats-section')).toBeVisible()
      await expect(page.locator('.stat-card')).toHaveCount(4)
    })

    test('should display risks and warnings', async ({ page }) => {
      await expect(page.locator('.risks-section')).toBeVisible()
      await expect(page.locator('.warnings-section')).toBeVisible()
    })
  })

  test.describe('Error Handling', () => {
    test('should display error message when API fails', async ({ page }) => {
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

    test('should not crash when switching tabs with error state', async ({ page }) => {
      await mockBeadsApiError(page)

      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      await page.selectOption('#project-select', '/code/test-project')

      // Wait a bit for API errors
      await page.waitForTimeout(500)

      // Switch between tabs - should not crash
      await page.click('.beads-subtab:has-text("Triage")')
      await expect(page.locator('.beads-view')).toBeVisible()

      await page.click('.beads-subtab:has-text("Insights")')
      await expect(page.locator('.beads-view')).toBeVisible()

      await page.click('.beads-subtab:has-text("Kanban")')
      await expect(page.locator('.beads-view')).toBeVisible()
    })

    test('should maintain beads-view element after error', async ({ page }) => {
      await mockBeadsApiError(page)

      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      await page.selectOption('#project-select', '/code/test-project')

      // Wait for API response
      await page.waitForTimeout(500)

      // BeadsView should still be present (not blank screen)
      await expect(page.locator('.beads-view')).toBeVisible()
      await expect(page.locator('.beads-header')).toBeVisible()
      await expect(page.locator('.beads-subtabs')).toBeVisible()
    })
  })

  test.describe('Refresh Button', () => {
    test('should show refresh button after selecting project', async ({ page }) => {
      await page.click('.tab:has-text("Beads")')
      await page.waitForSelector('#project-select')
      await page.selectOption('#project-select', '/code/test-project')

      await expect(page.locator('.beads-refresh-btn')).toBeVisible()
    })

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
