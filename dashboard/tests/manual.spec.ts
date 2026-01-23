import { test, expect } from '@playwright/test'
import { mockApiRoutes } from './mock-api'

test.describe('Manual View', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test.describe('Tab Navigation', () => {
    test('should switch to Manual tab', async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await expect(page.locator('.help-view')).toBeVisible()
    })

    test('should show Manual tab as active when selected', async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await expect(page.locator('.tab:has-text("Manual")')).toHaveClass(/active/)
    })

    test('should hide session panel when on Manual tab', async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await expect(page.locator('.session-panel')).not.toBeVisible()
    })

    test('should return to Terminal tab from Manual', async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await expect(page.locator('.help-view')).toBeVisible()

      await page.click('.tab:has-text("Terminal")')
      await expect(page.locator('.session-panel')).toBeVisible()
      await expect(page.locator('.terminal-area')).toBeVisible()
    })
  })

  test.describe('Content Rendering', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await page.waitForSelector('.help-view')
    })

    test('should display header with title', async ({ page }) => {
      await expect(page.locator('.help-header')).toBeVisible()
      await expect(page.locator('.help-title')).toContainText("Operator's Manual")
    })

    test('should display subtitle', async ({ page }) => {
      await expect(page.locator('.help-subtitle')).toContainText('Gas Town documentation')
    })

    test('should display navigation with 5 sections', async ({ page }) => {
      await expect(page.locator('.help-nav')).toBeVisible()
      await expect(page.locator('.help-nav-item')).toHaveCount(5)
    })

    test('should display section labels correctly', async ({ page }) => {
      await expect(page.locator('.help-nav-label:has-text("Overview")')).toBeVisible()
      await expect(page.locator('.help-nav-label:has-text("Getting Started")')).toBeVisible()
      await expect(page.locator('.help-nav-label:has-text("Roles")')).toBeVisible()
      await expect(page.locator('.help-nav-label:has-text("Commands")')).toBeVisible()
      await expect(page.locator('.help-nav-label:has-text("Workflows")')).toBeVisible()
    })

    test('should display section icons', async ({ page }) => {
      const icons = page.locator('.help-nav-icon')
      await expect(icons).toHaveCount(5)
      await expect(icons.nth(0)).toContainText('01')
      await expect(icons.nth(1)).toContainText('02')
      await expect(icons.nth(2)).toContainText('03')
      await expect(icons.nth(3)).toContainText('04')
      await expect(icons.nth(4)).toContainText('05')
    })

    test('should display content area', async ({ page }) => {
      await expect(page.locator('.help-content')).toBeVisible()
    })
  })

  test.describe('Section Navigation', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await page.waitForSelector('.help-view')
    })

    test('should show Overview section by default', async ({ page }) => {
      // Overview nav item should be active
      await expect(page.locator('.help-nav-item:has-text("Overview")')).toHaveClass(/active/)
      // Overview content should be visible
      await expect(page.locator('.help-hero')).toBeVisible()
      await expect(page.locator('.help-hero h2')).toContainText('Gas Town')
    })

    test('should switch to Getting Started section', async ({ page }) => {
      await page.click('.help-nav-item:has-text("Getting Started")')

      // Getting Started nav should be active
      await expect(page.locator('.help-nav-item:has-text("Getting Started")')).toHaveClass(/active/)
      // Overview should not be active
      await expect(page.locator('.help-nav-item:has-text("Overview")')).not.toHaveClass(/active/)
      // Content should change
      await expect(page.locator('.help-section-content h2')).toContainText('Getting Started')
    })

    test('should switch to Roles section', async ({ page }) => {
      await page.click('.help-nav-item:has-text("Roles")')

      await expect(page.locator('.help-nav-item:has-text("Roles")')).toHaveClass(/active/)
      await expect(page.locator('.help-section-content h2')).toContainText('Agent Roles')
    })

    test('should switch to Commands section', async ({ page }) => {
      await page.click('.help-nav-item:has-text("Commands")')

      await expect(page.locator('.help-nav-item:has-text("Commands")')).toHaveClass(/active/)
      await expect(page.locator('.help-section-content h2')).toContainText('Command Reference')
    })

    test('should switch to Workflows section', async ({ page }) => {
      await page.click('.help-nav-item:has-text("Workflows")')

      await expect(page.locator('.help-nav-item:has-text("Workflows")')).toHaveClass(/active/)
      await expect(page.locator('.help-section-content h2')).toContainText('Workflows')
    })

    test('should maintain section state when rapidly clicking', async ({ page }) => {
      // Rapidly click through sections
      await page.click('.help-nav-item:has-text("Getting Started")')
      await page.click('.help-nav-item:has-text("Roles")')
      await page.click('.help-nav-item:has-text("Commands")')
      await page.click('.help-nav-item:has-text("Workflows")')
      await page.click('.help-nav-item:has-text("Overview")')

      // Should end up on Overview
      await expect(page.locator('.help-nav-item:has-text("Overview")')).toHaveClass(/active/)
      await expect(page.locator('.help-hero')).toBeVisible()
    })
  })

  test.describe('Overview Section Content', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await page.waitForSelector('.help-view')
    })

    test('should display feature cards', async ({ page }) => {
      await expect(page.locator('.help-features-grid')).toBeVisible()
      await expect(page.locator('.help-feature-card')).toHaveCount(4)
    })

    test('should display feature card titles', async ({ page }) => {
      await expect(page.locator('.help-feature-card h3:has-text("Rigs")')).toBeVisible()
      await expect(page.locator('.help-feature-card h3:has-text("Polecats")')).toBeVisible()
      await expect(page.locator('.help-feature-card h3:has-text("Beads")')).toBeVisible()
      await expect(page.locator('.help-feature-card h3:has-text("Mail")')).toBeVisible()
    })

    test('should display architecture section', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Architecture")')).toBeVisible()
      await expect(page.locator('.help-list')).toBeVisible()
    })
  })

  test.describe('Getting Started Section Content', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await page.waitForSelector('.help-view')
      await page.click('.help-nav-item:has-text("Getting Started")')
    })

    test('should display essential commands', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Essential Commands")')).toBeVisible()
      await expect(page.locator('.help-shortcuts-table')).toBeVisible()
    })

    test('should display command examples', async ({ page }) => {
      await expect(page.locator('code:has-text("gt status")')).toBeVisible()
      await expect(page.locator('code:has-text("gt hook")')).toBeVisible()
      await expect(page.locator('code:has-text("gt mail inbox")')).toBeVisible()
    })

    test('should display workflow steps', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Workflow")')).toBeVisible()
      await expect(page.locator('.help-steps')).toBeVisible()
      await expect(page.locator('.help-step-number')).toHaveCount(3)
    })
  })

  test.describe('Roles Section Content', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await page.waitForSelector('.help-view')
      await page.click('.help-nav-item:has-text("Roles")')
    })

    test('should display town-level agents', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Town-Level Agents")')).toBeVisible()
      await expect(page.locator('.help-control-item:has-text("Mayor")')).toBeVisible()
      await expect(page.locator('.help-control-item:has-text("Deacon")')).toBeVisible()
    })

    test('should display rig-level agents', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Rig-Level Agents")')).toBeVisible()
      await expect(page.locator('.help-control-item:has-text("Witness")')).toBeVisible()
      await expect(page.locator('.help-control-item:has-text("Refinery")')).toBeVisible()
      await expect(page.locator('.help-control-item:has-text("Polecat")')).toBeVisible()
      await expect(page.locator('.help-control-item:has-text("Crew")')).toBeVisible()
    })
  })

  test.describe('Commands Section Content', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await page.waitForSelector('.help-view')
      await page.click('.help-nav-item:has-text("Commands")')
    })

    test('should display work management commands', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Work Management")')).toBeVisible()
      await expect(page.locator('code:has-text("gt sling")')).toBeVisible()
      await expect(page.locator('code:has-text("gt done")')).toBeVisible()
    })

    test('should display communication commands', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Communication")')).toBeVisible()
      await expect(page.locator('code:has-text("gt nudge")')).toBeVisible()
      await expect(page.locator('code:has-text("gt broadcast")')).toBeVisible()
    })

    test('should display monitoring commands', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Monitoring")')).toBeVisible()
      await expect(page.locator('code:has-text("gt ready")')).toBeVisible()
      await expect(page.locator('code:has-text("gt trail")')).toBeVisible()
    })
  })

  test.describe('Workflows Section Content', () => {
    test.beforeEach(async ({ page }) => {
      await page.click('.tab:has-text("Manual")')
      await page.waitForSelector('.help-view')
      await page.click('.help-nav-item:has-text("Workflows")')
    })

    test('should display polecat work cycle', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Polecat Work Cycle")')).toBeVisible()
      await expect(page.locator('.help-steps')).toBeVisible()
    })

    test('should display convoy pattern', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Convoy Pattern")')).toBeVisible()
    })

    test('should display best practices', async ({ page }) => {
      await expect(page.locator('.help-card h3:has-text("Best Practices")')).toBeVisible()
      await expect(page.locator('.help-card-accent')).toBeVisible()
    })
  })

  test.describe('Tab Persistence', () => {
    test('should maintain Manual view state on tab switch and return', async ({ page }) => {
      // Go to Manual tab
      await page.click('.tab:has-text("Manual")')
      await page.waitForSelector('.help-view')

      // Navigate to Commands section
      await page.click('.help-nav-item:has-text("Commands")')
      await expect(page.locator('.help-nav-item:has-text("Commands")')).toHaveClass(/active/)

      // Switch to another tab
      await page.click('.tab:has-text("Files")')
      await expect(page.locator('.files-view')).toBeVisible()

      // Return to Manual tab - should be back at Overview (state not persisted)
      await page.click('.tab:has-text("Manual")')
      await expect(page.locator('.help-nav-item:has-text("Overview")')).toHaveClass(/active/)
    })
  })
})
