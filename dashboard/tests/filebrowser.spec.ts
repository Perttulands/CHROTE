import { test, expect, Page } from './fixtures'
import { mockLaunchApiRoute, mockThemeApiRoute, mockWorkspacesRoute } from './mock-api'

// Files behaviour lives in the unit suite; what is left here needs a real
// layout engine: a measured viewport box and a real scrolling container.

const fileResourcesPattern = /.*\/api\/files\/resources(?:\/.*)?$/

const mockDirectoryResponse = {
  isDir: true,
  items: [
    { name: 'code', size: 0, modified: '2024-01-15T10:00:00Z', isDir: true, type: '' },
    { name: 'projects', size: 0, modified: '2024-01-14T09:00:00Z', isDir: true, type: '' },
    { name: 'readme.txt', size: 1024, modified: '2024-01-13T08:00:00Z', isDir: false, type: 'text' },
  ],
}

async function mockFilebrowserApi(page: Page, options?: {
  rawBody?: string
  rootItems?: typeof mockDirectoryResponse.items
}) {
  // The empty window's launcher asks which workspaces exist, to offer them.
  await mockWorkspacesRoute(page)
  // Mock the tmux sessions API (required for dashboard to load)
  await page.route('**/api/tmux/sessions', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ sessions: [], grouped: {}, timestamp: new Date().toISOString() }),
    })
  })

  await mockThemeApiRoute(page)
  await mockLaunchApiRoute(page)

  await page.route('**/api/files/raw/**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'text/plain',
      body: options?.rawBody ?? 'mock file content',
    })
  })

  await page.route(fileResourcesPattern, async route => {
    const url = route.request().url()

    if (url.endsWith('/resources/') || url.endsWith('/resources')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          ...mockDirectoryResponse,
          items: options?.rootItems ?? mockDirectoryResponse.items,
        }),
      })
      return
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ isDir: true, items: [] }),
    })
  })
}

test.describe('Filebrowser layout regressions', () => {
  test('Markdown Source fills the artifact viewport instead of collapsing to textarea rows', async ({ page }) => {
    await page.setViewportSize({ width: 1200, height: 760 })
    await mockFilebrowserApi(page, {
      rootItems: [{ name: 'README.md', size: 4096, modified: '2026-07-13T00:00:00Z', isDir: false, type: 'text/markdown' }],
      rawBody: Array.from({ length: 160 }, (_, index) => `line ${index} ${'source '.repeat(24)}`).join('\n'),
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.click('.fb-row:has-text("README.md")')
    await page.getByRole('button', { name: 'Show Markdown source' }).click()

    const source = page.getByRole('textbox', { name: 'Markdown source for README.md' })
    const viewport = page.getByTestId('file-viewer-scroll')
    await expect(source).toHaveAttribute('wrap', 'off')
    const sourceBox = await source.boundingBox()
    const viewportBox = await viewport.boundingBox()
    expect(sourceBox).toBeTruthy()
    expect(viewportBox).toBeTruthy()
    expect(sourceBox!.width).toBeGreaterThanOrEqual(viewportBox!.width - 2)
    expect(sourceBox!.height).toBeGreaterThanOrEqual(viewportBox!.height - 2)
  })

  test('the explorer tree scrolls a folder too long to fit instead of stretching the page', async ({ page }) => {
    await mockFilebrowserApi(page, {
      rootItems: Array.from({ length: 80 }, (_, index) => ({
        name: `artifact-${String(index).padStart(2, '0')}.txt`,
        size: 128,
        modified: '2026-07-13T00:00:00Z',
        isDir: false,
        type: 'text/plain',
      })),
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')

    const tree = page.getByRole('tree', { name: 'File tree' })
    await expect(tree.getByRole('treeitem')).toHaveCount(80)

    const geometry = await tree.evaluate(element => ({
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      overflowY: getComputedStyle(element).overflowY,
    }))
    expect(geometry.scrollHeight).toBeGreaterThan(geometry.clientHeight)
    expect(geometry.overflowY).toMatch(/auto|scroll/)

    await tree.hover()
    await page.mouse.wheel(0, 700)
    await expect.poll(() => tree.evaluate(element => element.scrollTop)).toBeGreaterThan(0)
  })
})
