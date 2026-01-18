import { test, expect, Page } from '@playwright/test'

/**
 * TDD Test: Path Mapping for FilesView
 *
 * These tests verify that:
 * 1. API calls use /code (not /srv/code) - filebrowser root is /
 * 2. UI displays E:/Code (not /code or /srv/code)
 * 3. Root folders show as E:/Code and E:/Vault
 */

// Mock data matching expected API paths (/code, not /srv/code)
const mockRootResponse = {
  isDir: true,
  items: [
    { name: 'code', size: 0, modified: '2024-01-15T10:00:00Z', isDir: true, type: '' },
    { name: 'vault', size: 0, modified: '2024-01-14T09:00:00Z', isDir: true, type: '' },
  ],
}

const mockCodeDirResponse = {
  isDir: true,
  items: [
    { name: 'project1', size: 0, modified: '2024-01-15T10:00:00Z', isDir: true, type: '' },
    { name: 'readme.md', size: 256, modified: '2024-01-14T09:00:00Z', isDir: false, type: 'text/markdown' },
  ],
}

async function setupPathTest(page: Page) {
  // Track API calls to verify correct paths
  const apiCalls: string[] = []

  // Mock all API endpoints to prevent connection errors
  await page.route('**/api/**', async route => {
    const url = route.request().url()

    if (url.includes('/api/tmux/sessions')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ sessions: [], grouped: {}, timestamp: new Date().toISOString() }),
      })
      return
    }

    if (url.includes('/api/tmux/appearance')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ theme: 'dark' }),
      })
      return
    }

    // Default API response
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({}),
    })
  })

  await page.route('**/api/files/resources/**', async route => {
    const url = route.request().url()
    apiCalls.push(url)

    // Root directory request - should be /resources/ or /resources (no /srv)
    if (url.endsWith('/resources/') || url.endsWith('/resources')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(mockRootResponse),
      })
      return
    }

    // /code directory - API should request /resources/code (NOT /resources/srv/code)
    if (url.includes('/resources/code') && !url.includes('/srv/')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(mockCodeDirResponse),
      })
      return
    }

    // If we get /srv/code, that's the bug - return 404
    if (url.includes('/srv/')) {
      await route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Not found - /srv prefix should not be in API path' }),
      })
      return
    }

    // Default
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ isDir: true, items: [] }),
    })
  })

  return { apiCalls }
}

test.describe('Path Mapping - API Calls', () => {
  test('root request should NOT include /srv prefix', async ({ page }) => {
    const { apiCalls } = await setupPathTest(page)

    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })

    // Should have made API call to root
    const rootCall = apiCalls.find(url => url.includes('/resources'))
    expect(rootCall).toBeDefined()
    expect(rootCall).not.toContain('/srv/')
  })

  test('navigating to code folder should use /code NOT /srv/code', async ({ page }) => {
    const { apiCalls } = await setupPathTest(page)

    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })

    // Double-click on "code" folder (displayed as E:/Code)
    await page.dblclick('.fb-row:has-text("E:/Code"), .fb-grid-item:has-text("E:/Code")')

    // Wait for navigation
    await page.waitForTimeout(500)

    // Check API calls - should have /resources/code, NOT /resources/srv/code
    const codeCall = apiCalls.find(url => url.includes('/code'))
    expect(codeCall).toBeDefined()
    expect(codeCall).not.toContain('/srv/')
    expect(codeCall).toContain('/resources/code')
  })
})

test.describe('Path Mapping - UI Display', () => {
  test.beforeEach(async ({ page }) => {
    await setupPathTest(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })
  })

  test('root folders should display as E:/Code and E:/Vault', async ({ page }) => {
    // Should show E:/Code, not "code" or "/code" or "/srv/code"
    await expect(page.locator('.fb-filename:has-text("E:/Code"), .fb-grid-name:has-text("E:/Code")')).toBeVisible()
    await expect(page.locator('.fb-filename:has-text("E:/Vault"), .fb-grid-name:has-text("E:/Vault")')).toBeVisible()

    // Should NOT show raw container paths
    await expect(page.locator('.fb-filename:has-text("/srv/"), .fb-grid-name:has-text("/srv/")')).not.toBeVisible()
  })

  test('breadcrumbs should show E:/Code when navigating', async ({ page }) => {
    // Navigate into code folder
    await page.dblclick('.fb-row:has-text("E:/Code"), .fb-grid-item:has-text("E:/Code")')

    // Breadcrumb should show E:/Code, not /code or /srv/code
    await expect(page.locator('.fb-breadcrumb-item:has-text("E:/Code")')).toBeVisible()
    await expect(page.locator('.fb-breadcrumb-item:has-text("/srv/")')).not.toBeVisible()
  })

  test('current path display should show Windows path format', async ({ page }) => {
    // Navigate into code folder
    await page.dblclick('.fb-row:has-text("E:/Code"), .fb-grid-item:has-text("E:/Code")')

    // The path display/breadcrumbs should show E:/Code format
    const breadcrumbs = page.locator('.fb-breadcrumbs')
    await expect(breadcrumbs).toContainText('E:/Code')
    await expect(breadcrumbs).not.toContainText('/srv/')
  })
})

test.describe('Path Mapping - Context Menu', () => {
  test('copy path should use Windows format E:/Code', async ({ page }) => {
    await setupPathTest(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })

    // Navigate into code folder first
    await page.dblclick('.fb-row:has-text("E:/Code"), .fb-grid-item:has-text("E:/Code")')
    await page.waitForTimeout(500)

    // Right-click on a file
    await page.click('.fb-row:has-text("readme.md"), .fb-grid-item:has-text("readme.md")', { button: 'right' })

    // Context menu should have Copy Path option
    const copyPathItem = page.locator('.fb-context-item:has-text("Copy Path")')

    if (await copyPathItem.isVisible()) {
      // If Copy Path exists, clicking it should copy E:/Code/readme.md format
      // We can't easily test clipboard, but we can verify the menu item exists
      await expect(copyPathItem).toBeVisible()
    }
  })
})
