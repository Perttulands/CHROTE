import { test, expect, allowBrowserConsoleMessage, Page } from './fixtures'

const fileResourcesPattern = /.*\/api\/files\/resources(?:\/.*)?$/

// Mock filebrowser API responses
const mockDirectoryResponse = {
  isDir: true,
  items: [
    { name: 'code', size: 0, modified: '2024-01-15T10:00:00Z', isDir: true, type: '' },
    { name: 'projects', size: 0, modified: '2024-01-14T09:00:00Z', isDir: true, type: '' },
    { name: 'readme.txt', size: 1024, modified: '2024-01-13T08:00:00Z', isDir: false, type: 'text' },
  ],
}

const mockSubdirectoryResponse = {
  isDir: true,
  items: [
    { name: 'src', size: 0, modified: '2024-01-15T10:00:00Z', isDir: true, type: '' },
    { name: 'package.json', size: 512, modified: '2024-01-14T09:00:00Z', isDir: false, type: 'application/json' },
  ],
}

async function mockFilebrowserApi(page: Page, options?: { failConnection?: boolean; delay?: number }) {
  // Mock the tmux sessions API (required for dashboard to load)
  await page.route('**/api/tmux/sessions', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ sessions: [], grouped: {}, timestamp: new Date().toISOString() }),
    })
  })

  await page.route('**/api/tmux/appearance', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true }),
    })
  })

  await page.route('**/api/files/raw/**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'text/plain',
      body: 'mock file content',
    })
  })

  // Mock filebrowser API
  await page.route(fileResourcesPattern, async route => {
    if (options?.failConnection) {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'file service unavailable' }),
      })
      return
    }

    if (options?.delay) {
      await new Promise(resolve => setTimeout(resolve, options.delay))
    }

    const url = route.request().url()

    // Root directory
    if (url.endsWith('/resources/') || url.endsWith('/resources')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(mockDirectoryResponse),
      })
      return
    }

    // /code subdirectory
    if (url.includes('/resources/code')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(mockSubdirectoryResponse),
      })
      return
    }

    // Default: return empty directory
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ isDir: true, items: [] }),
    })
  })
}

test.describe('Filebrowser Connection', () => {
  test('should show loading state while fetching directory', async ({ page }) => {
    // Add delay to observe loading state
    await mockFilebrowserApi(page, { delay: 1500 })

    await page.goto('/')
    await page.waitForSelector('.dashboard')

    // Switch to Files tab
    await page.click('.tab:has-text("Files")')

    // Should show loading indicator
    await expect(page.locator('.fb-loading')).toBeVisible()
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })
  })

  test('should load and display directory contents', async ({ page }) => {
    await mockFilebrowserApi(page)

    await page.goto('/')
    await page.waitForSelector('.dashboard')

    // Switch to Files tab
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')

    // Wait for loading to complete
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })

    // Should display files from mock
    await expect(page.locator('.fb-row, .fb-grid-item')).toHaveCount(3)
    await expect(page.locator('.fb-filename:has-text("code"), .fb-grid-name:has-text("code")')).toBeVisible()
    await expect(page.locator('.fb-filename:has-text("projects"), .fb-grid-name:has-text("projects")')).toBeVisible()
    await expect(page.locator('.fb-filename:has-text("readme.txt"), .fb-grid-name:has-text("readme.txt")')).toBeVisible()
  })

  test('should show error state when connection fails', async ({ page }) => {
    allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 503')
    await mockFilebrowserApi(page, { failConnection: true })

    await page.goto('/')
    await page.waitForSelector('.dashboard')

    // Switch to Files tab
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')

    // Should show error state
    await expect(page.locator('.fb-error')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('.fb-retry-btn')).toBeVisible()
  })

  test('should retry loading on retry button click', async ({ page }) => {
    allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 503')
    // First: set up failure state
    await mockFilebrowserApi(page, { failConnection: true })

    await page.goto('/')
    await page.waitForSelector('.dashboard')

    // Switch to Files tab - should show error
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
    await expect(page.locator('.fb-error')).toBeVisible({ timeout: 5000 })
    await expect(page.locator('.fb-retry-btn')).toBeVisible()

    // Now set up success state for retry
    await page.unroute(fileResourcesPattern)
    await page.route(fileResourcesPattern, async route => {
      const url = route.request().url()
      if (url.endsWith('/resources/') || url.endsWith('/resources')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(mockDirectoryResponse),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ isDir: true, items: [] }),
        })
      }
    })

    // Click retry
    await page.click('.fb-retry-btn')

    // Should now show content
    await expect(page.locator('.fb-error')).not.toBeVisible({ timeout: 5000 })
    await expect(page.locator('.fb-row, .fb-grid-item')).toHaveCount(3)
  })
})

test.describe('Filebrowser Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await mockFilebrowserApi(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })
  })

  test('should navigate into folder on double-click', async ({ page }) => {
    // Double-click on "code" folder
    await page.dblclick('.fb-row:has-text("code"), .fb-grid-item:has-text("code")')

    // Should show breadcrumb with "code"
    await expect(page.locator('.fb-breadcrumb-item:has-text("code")')).toBeVisible()

    // Should show contents of code directory
    await expect(page.locator('.fb-filename:has-text("src"), .fb-grid-name:has-text("src")')).toBeVisible()
    await expect(page.locator('.fb-filename:has-text("package.json"), .fb-grid-name:has-text("package.json")')).toBeVisible()
  })

  test('should navigate back using breadcrumbs', async ({ page }) => {
    // Navigate into code folder
    await page.dblclick('.fb-row:has-text("code"), .fb-grid-item:has-text("code")')
    await expect(page.locator('.fb-breadcrumb-item:has-text("code")')).toBeVisible()

    // Click root breadcrumb
    await page.click('.fb-breadcrumb-root')

    // Should be back at root
    await expect(page.locator('.fb-filename:has-text("code"), .fb-grid-name:has-text("code")')).toBeVisible()
    await expect(page.locator('.fb-row, .fb-grid-item')).toHaveCount(3)
  })

  test('should navigate up using up button', async ({ page }) => {
    // Navigate into code folder
    await page.dblclick('.fb-row:has-text("code"), .fb-grid-item:has-text("code")')
    await expect(page.locator('.fb-breadcrumb-item:has-text("code")')).toBeVisible()

    // Click up button
    await page.click('.fb-nav-btn[title="Up"]')

    // Should be back at root
    await expect(page.locator('.fb-filename:has-text("code"), .fb-grid-name:has-text("code")')).toBeVisible()
  })

  test('should refresh directory on refresh button click', async ({ page }) => {
    // Get current item count
    const initialCount = await page.locator('.fb-row, .fb-grid-item').count()
    expect(initialCount).toBe(3)

    // Click refresh
    await page.click('.fb-btn[title="Refresh"]')

    // Wait for content to reload (items should still be there)
    await expect(page.locator('.fb-row, .fb-grid-item')).toHaveCount(3, { timeout: 3000 })
  })
})

test.describe('Filebrowser UI Elements', () => {
  test.beforeEach(async ({ page }) => {
    await mockFilebrowserApi(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })
  })

  test('should switch between list and grid view', async ({ page }) => {
    // Default is list view
    await expect(page.locator('.fb-list')).toBeVisible()

    // Switch to grid view
    await page.click('.fb-view-btn[title="Grid view"]')
    await expect(page.locator('.fb-grid')).toBeVisible()
    await expect(page.locator('.fb-list')).not.toBeVisible()

    // Switch back to list view
    await page.click('.fb-view-btn[title="List view"]')
    await expect(page.locator('.fb-list')).toBeVisible()
  })

  test('should filter files by search', async ({ page }) => {
    // Type in filter
    await page.fill('.fb-search', 'code')

    // Should only show matching items
    await expect(page.locator('.fb-row, .fb-grid-item')).toHaveCount(1)
    await expect(page.locator('.fb-filename:has-text("code"), .fb-grid-name:has-text("code")')).toBeVisible()
  })

  test('should show item count in status bar', async ({ page }) => {
    await expect(page.locator('.fb-statusbar')).toContainText('3 items')
  })

  test('should select item on click', async ({ page }) => {
    await page.click('.fb-row:has-text("readme.txt")')

    // Item should be selected
    await expect(page.locator('.fb-row.selected')).toHaveCount(1)

    // Status bar should show selection
    await expect(page.locator('.fb-statusbar')).toContainText('1 selected')
  })

  test('should show context menu on right-click', async ({ page }) => {
    // Right-click on a file
    await page.click('.fb-row:has-text("readme.txt")', { button: 'right' })

    // Context menu should appear
    await expect(page.locator('.fb-context-menu')).toBeVisible()
    await expect(page.locator('.fb-context-item:has-text("Download")')).toBeVisible()
    await expect(page.locator('.fb-context-item:has-text("Rename")')).toBeVisible()
    await expect(page.locator('.fb-context-item:has-text("Delete")')).toBeVisible()
  })

  test('should close context menu on click outside', async ({ page }) => {
    // Open context menu
    await page.click('.fb-row:has-text("readme.txt")', { button: 'right' })
    await expect(page.locator('.fb-context-menu')).toBeVisible()

    // Click outside
    await page.click('.fb-list-container')

    // Context menu should close
    await expect(page.locator('.fb-context-menu')).not.toBeVisible()
  })

  test('should sort by column headers', async ({ page }) => {
    // Click on Size header to sort
    await page.click('.fb-column-header:has-text("Size")')

    // Should show sort indicator
    await expect(page.locator('.fb-column-header:has-text("Size")')).toHaveClass(/active/)
  })
})

test.describe('Filebrowser Workbench', () => {
  test.beforeEach(async ({ page }) => {
    await mockFilebrowserApi(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
    await expect(page.locator('.fb-loading')).not.toBeVisible({ timeout: 5000 })
  })

  test('should display explorer tree and preview pane', async ({ page }) => {
    await expect(page.locator('.fb-sidebar')).toBeVisible()
    await expect(page.locator('.fb-editor-pane')).toBeVisible()
    await expect(page.locator('.fb-section-title:has-text("Workspace")')).toBeVisible()
    await expect(page.locator('.fb-editor-empty')).toContainText('No file selected')
  })

  test('should open a text file in the editor pane', async ({ page }) => {
    await page.click('.fb-row:has-text("readme.txt")')

    await expect(page.locator('.fb-editor-tab:has-text("readme.txt")')).toBeVisible()
    await expect(page.locator('.fb-editor-textarea')).toHaveValue('mock file content')
  })

  test('should have upload control in the toolbar', async ({ page }) => {
    await page.dblclick('.fb-row:has-text("code")')
    await expect(page.locator('.fb-btn[title="Upload"]')).toBeEnabled()
    await expect(page.locator('.fb-hidden-input[type="file"]')).toHaveCount(1)
  })

  test('should upload files to the current folder', async ({ page }) => {
    const uploadRequests: { url: string; body: Buffer }[] = []
    let uploadCompleted = false
    let postUploadRefreshCompleted = false

    await page.route(fileResourcesPattern, async route => {
      const request = route.request()
      if (request.method() === 'POST') {
        uploadRequests.push({
          url: request.url(),
          body: request.postDataBuffer() || Buffer.from(''),
        })
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true }),
        })
        uploadCompleted = true
        return
      }

      const isPostUploadRefresh = uploadCompleted && request.url().includes('/resources/code')
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(request.url().includes('/resources/code')
          ? mockSubdirectoryResponse
          : mockDirectoryResponse),
      })
      if (isPostUploadRefresh) {
        postUploadRefreshCompleted = true
      }
    })

    await page.dblclick('.fb-row:has-text("code")')
    await expect(page.locator('.fb-row, .fb-grid-item')).toHaveCount(2)

    const fileInput = page.locator('.fb-hidden-input[type="file"]')
    await fileInput.setInputFiles({
      name: 'upload.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('uploaded content'),
    })

    await expect(async () => {
      expect(uploadRequests.length).toBe(1)
    }).toPass({ timeout: 3000 })
    await expect(async () => {
      expect(postUploadRefreshCompleted).toBe(true)
    }).toPass({ timeout: 3000 })
    expect(uploadRequests[0].url).toContain('/resources/code/upload.txt')
    expect(uploadRequests[0].body.toString()).toBe('uploaded content')
  })
})

test.describe('Filebrowser Tab Navigation', () => {
  test.beforeEach(async ({ page }) => {
    await mockFilebrowserApi(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test('should switch to Files tab and back to Terminal', async ({ page }) => {
    // Initially on Terminal tab
    await expect(page.locator('.session-panel')).toBeVisible()

    // Switch to Files
    await page.click('.tab:has-text("Files")')
    await expect(page.locator('.files-view')).toBeVisible()
    await expect(page.locator('.session-panel')).not.toBeVisible()

    // Switch back to Terminal
    await page.click('.tab:has-text("Terminal")')
    await expect(page.locator('.session-panel')).toBeVisible()
    await expect(page.locator('.files-view')).not.toBeVisible()
  })

  test('should show editor pane content', async ({ page }) => {
    // Go to Files tab
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')

    await expect(page.locator('.fb-editor-pane')).toBeVisible()
    await expect(page.locator('.fb-editor-empty')).toContainText('No file selected')

    await page.click('.fb-row:has-text("readme.txt")')
    await expect(page.locator('.fb-editor-tab:has-text("readme.txt")')).toBeVisible()
  })
})
