import { test, expect, Page } from './fixtures'
import { mockBeadsProjectsRoute, mockLaunchApiRoute, mockPersistentTabApiRoutes, mockThemeApiRoute, mockWorkspacesRoute } from './mock-api'

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
  await mockBeadsProjectsRoute(page)
  await mockPersistentTabApiRoutes(page)
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

  // Typing at home also finds files everywhere beneath the projects.
  await page.route('**/api/files/find*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ matches: [], truncated: false }),
  }))

  await page.route('**/api/files/diff*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ path: '', repository: '', diff: '', truncated: false }),
  }))

  await page.route(fileResourcesPattern, async route => {
    const url = route.request().url()

    // The roots: one, /work, holding what the test lists.
    if (url.endsWith('/resources/') || url.endsWith('/resources')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ isDir: true, items: [{ name: 'work', size: 0, modified: '2024-01-15T10:00:00Z', isDir: true, type: '' }] }),
      })
      return
    }
    if (url.endsWith('/resources/work') || url.endsWith('/resources/work/')) {
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

test.describe('The Files tab', () => {
  test('opens on the projects, and a project lands beside the column', async ({ page }) => {
    await page.setViewportSize({ width: 1200, height: 760 })
    await mockFilebrowserApi(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')

    const code = page.getByRole('list', { name: '/code' })
    await expect(code.getByRole('listitem')).toHaveCount(3)
    await expect(page.getByRole('list', { name: '/srv' }).getByRole('listitem', { name: /chrote/ })).toContainText('1 live')
    await page.getByRole('searchbox', { name: 'Search projects' }).fill('quiet')
    await expect(code.getByRole('listitem')).toHaveCount(1)
    await page.keyboard.press('Enter')

    const landing = page.getByRole('article', { name: 'Project quiet-project' })
    await expect(landing).toBeVisible()
    await expect(page.getByRole('searchbox', { name: 'Search files in quiet-project' })).toBeVisible()
    const [panelBox, landingBox] = [
      (await page.getByRole('complementary', { name: 'Files', exact: true }).boundingBox())!,
      (await landing.boundingBox())!,
    ]
    expect(landingBox.x).toBeGreaterThanOrEqual(panelBox.x + panelBox.width - 1)
  })

  test('reads a file beside the tree, filling the rest of the tab', async ({ page }) => {
    await page.setViewportSize({ width: 1200, height: 760 })
    await mockFilebrowserApi(page, {
      rootItems: [{ name: 'notes.txt', size: 4096, modified: '2026-07-13T00:00:00Z', isDir: false, type: 'text/plain' }],
      rawBody: Array.from({ length: 160 }, (_, index) => `line ${index}`).join('\n'),
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')
    await page.click('.tab:has-text("Files")')
    await page.getByRole('list', { name: 'Roots' }).getByRole('listitem', { name: '/work' }).click()
    await page.locator('.files-tab .fb-tree-row', { hasText: 'notes.txt' }).click()

    const panel = page.getByRole('complementary', { name: 'Files', exact: true })
    const viewer = page.getByRole('region', { name: 'File' })
    await expect(viewer).toContainText('line 159')
    const [panelBox, viewerBox] = [(await panel.boundingBox())!, (await viewer.boundingBox())!]
    expect(viewerBox.x).toBeGreaterThanOrEqual(panelBox.x + panelBox.width - 1)
    expect(viewerBox.x + viewerBox.width).toBeGreaterThanOrEqual(1200 - 1)
    // No pop-out: the file is the tab's, not a window over it.
    await expect(page.getByRole('dialog', { name: 'File notes.txt' })).toHaveCount(0)
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
    await page.getByRole('list', { name: 'Roots' }).getByRole('listitem', { name: '/work' }).click()

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
