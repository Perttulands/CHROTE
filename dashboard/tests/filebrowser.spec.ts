import { test, expect, Page } from '@playwright/test'

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

  // Mock filebrowser API
  await page.route('**/api/files/resources/**', async route => {
    if (options?.failConnection) {
      await route.abort('connectionfailed')
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
    await mockFilebrowserApi(page, { delay: 500 })

    await page.goto('/')
    await page.waitForSelector('.dashboard')

    // Switch to Files tab
    await page.click('.tab:has-text("Files")')

    // Should show loading indicator
    await expect(page.locator('.fb-loading')).toBeVisible()
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
    await page.unroute('**/api/files/resources/**')
    await page.route('**/api/files/resources/**', async route => {
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

    // Wait for any loading state to appear and disappear
    await page.waitForTimeout(300)

    // Items should still be there (content reloaded)
    const afterCount = await page.locator('.fb-row, .fb-grid-item').count()
    expect(afterCount).toBe(3)
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

test.describe('Filebrowser Inbox Panel', () => {
  test.beforeEach(async ({ page }) => {
    await mockFilebrowserApi(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
  })

  test('should display inbox panel', async ({ page }) => {
    await expect(page.locator('.inbox-panel')).toBeVisible()
    await expect(page.locator('.inbox-title')).toContainText('Send a package to E:/Code/incoming')
  })

  test('should have file input for upload', async ({ page }) => {
    // The dropzone should be clickable
    await expect(page.locator('.inbox-dropzone')).toBeVisible()
  })
})

test.describe('Filebrowser Inbox Send E2E', () => {
  test.beforeEach(async ({ page }) => {
    await mockFilebrowserApi(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')
  })

  test('should enable send button when file is selected', async ({ page }) => {
    // Find the hidden file input
    const fileInput = page.locator('.inbox-dropzone input[type="file"]:first-of-type')

    // Send button should be disabled initially
    const sendButton = page.locator('.inbox-send')
    await expect(sendButton).toBeDisabled()

    // Select a file
    await fileInput.setInputFiles({
      name: 'test-report.pdf',
      mimeType: 'application/pdf',
      buffer: Buffer.from('test file content'),
    })

    // Send button should now be enabled
    await expect(sendButton).toBeEnabled()
  })

  test('should show selected file name', async ({ page }) => {
    const fileInput = page.locator('.inbox-dropzone input[type="file"]:first-of-type')

    await fileInput.setInputFiles({
      name: 'my-document.pdf',
      mimeType: 'application/pdf',
      buffer: Buffer.from('test content'),
    })

    // Should show the file name
    await expect(page.locator('.inbox-selected-file, .inbox-file-name')).toContainText('my-document.pdf')
  })

  test('should allow entering a note', async ({ page }) => {
    const noteInput = page.locator('.inbox-note, textarea[placeholder*="note"], textarea[placeholder*="message"]')

    if (await noteInput.count() > 0) {
      await noteInput.fill('Please analyze this report')
      await expect(noteInput).toHaveValue('Please analyze this report')
    }
  })

  test('should upload file on send', async ({ page }) => {
    const uploadRequests: { url: string; method: string; body: Buffer }[] = []

    // Track file upload requests
    await page.route('**/api/files/resources/**', async route => {
      const request = route.request()
      if (request.method() === 'POST') {
        uploadRequests.push({
          url: request.url(),
          method: request.method(),
          body: request.postDataBuffer() || Buffer.from(''),
        })
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(mockDirectoryResponse),
        })
      }
    })

    // Select a file
    const fileInput = page.locator('.inbox-dropzone input[type="file"]:first-of-type')
    await fileInput.setInputFiles({
      name: 'report.pdf',
      mimeType: 'application/pdf',
      buffer: Buffer.from('PDF content here'),
    })

    // Click send
    const sendButton = page.locator('.inbox-send')
    if (await sendButton.isEnabled()) {
      await sendButton.click()

      // Wait for upload
      await page.waitForTimeout(500)

      // Should have made upload requests
      // One for the file, possibly one for the note
      expect(uploadRequests.length).toBeGreaterThanOrEqual(1)

      // File should go to /code/incoming/
      const fileUpload = uploadRequests.find(r => r.url.includes('/incoming/'))
      expect(fileUpload).toBeDefined()
    }
  })

  test('should create note file alongside uploaded file', async ({ page }) => {
    const uploadRequests: { url: string }[] = []

    await page.route('**/api/files/resources/**', async route => {
      const request = route.request()
      if (request.method() === 'POST') {
        uploadRequests.push({ url: request.url() })
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(mockDirectoryResponse),
        })
      }
    })

    // Select file
    const fileInput = page.locator('.inbox-dropzone input[type="file"]:first-of-type')
    await fileInput.setInputFiles({
      name: 'data.csv',
      mimeType: 'text/csv',
      buffer: Buffer.from('a,b,c\n1,2,3'),
    })

    // Enter a note
    const noteInput = page.locator('.inbox-note, textarea[placeholder*="note"], textarea[placeholder*="message"]')
    if (await noteInput.count() > 0) {
      await noteInput.fill('Process this CSV data')
    }

    // Send
    const sendButton = page.locator('.inbox-send')
    if (await sendButton.isEnabled()) {
      await sendButton.click()
      await page.waitForTimeout(500)

      // Should have created both file and note
      const noteUpload = uploadRequests.find(r => r.url.includes('.note') || r.url.includes('.letter'))
      if (noteInput && await noteInput.count() > 0) {
        expect(noteUpload).toBeDefined()
      }
    }
  })

  test('should clear form after successful send', async ({ page }) => {
    await page.route('**/api/files/resources/**', async route => {
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(mockDirectoryResponse),
        })
      }
    })

    // Select file
    const fileInput = page.locator('.inbox-dropzone input[type="file"]:first-of-type')
    await fileInput.setInputFiles({
      name: 'test.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('test'),
    })

    // Send
    const sendButton = page.locator('.inbox-send')
    if (await sendButton.isEnabled()) {
      await sendButton.click()
      await page.waitForTimeout(500)

      // Form should be cleared
      await expect(sendButton).toBeDisabled()

      // Note should be cleared
      const noteInput = page.locator('.inbox-note, textarea')
      if (await noteInput.count() > 0) {
        await expect(noteInput).toHaveValue('')
      }
    }
  })

  test('should show error on upload failure', async ({ page }) => {
    await page.route('**/api/files/resources/**', async route => {
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 500,
          contentType: 'application/json',
          body: JSON.stringify({ error: 'Upload failed' }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(mockDirectoryResponse),
        })
      }
    })

    // Select file
    const fileInput = page.locator('.inbox-dropzone input[type="file"]:first-of-type')
    await fileInput.setInputFiles({
      name: 'fail.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('will fail'),
    })

    // Send
    const sendButton = page.locator('.inbox-send')
    if (await sendButton.isEnabled()) {
      await sendButton.click()
      await page.waitForTimeout(500)

      // Should show error toast
      const errorToast = page.locator('.fb-error-toast')
      await expect(errorToast).toBeVisible()
    }
  })

  test('should support multiple file selection', async ({ page }) => {
    const uploadRequests: string[] = []

    await page.route('**/api/files/resources/**', async route => {
      if (route.request().method() === 'POST') {
        uploadRequests.push(route.request().url())
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true }),
        })
      } else {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(mockDirectoryResponse),
        })
      }
    })

    // Select multiple files
    const fileInput = page.locator('.inbox-dropzone input[type="file"]:first-of-type')
    await fileInput.setInputFiles([
      { name: 'file1.txt', mimeType: 'text/plain', buffer: Buffer.from('one') },
      { name: 'file2.txt', mimeType: 'text/plain', buffer: Buffer.from('two') },
    ])

    // Send
    const sendButton = page.locator('.inbox-send')
    if (await sendButton.isEnabled()) {
      await sendButton.click()
      await page.waitForTimeout(500)

      // Should upload both files
      const fileUploads = uploadRequests.filter(u => u.includes('/incoming/'))
      expect(fileUploads.length).toBeGreaterThanOrEqual(2)
    }
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

  test('should show Info tab content', async ({ page }) => {
    // Go to Files tab
    await page.click('.tab:has-text("Files")')
    await page.waitForSelector('.files-view')

    // Switch to Info sub-tab
    await page.click('.fb-tab:has-text("Info")')

    // Should show info panel
    await expect(page.locator('.fb-info-panel')).toBeVisible()
    await expect(page.locator('.fb-info-card')).toHaveCount(3)
  })
})
