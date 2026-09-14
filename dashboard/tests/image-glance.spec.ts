import { test, expect, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * The image glance (bead: chrote-5grx.45).
 *
 * The glance is the Files tab's look at a picture: a click on the picture in
 * the tab's viewer opens it centred over the workspace, Escape and a press
 * outside close it, a corner dragged there is the size every later glance
 * opens at (bead: chrote-dx3r), and the zoom level a step sets is the level
 * the next picture opens at (bead: chrote-4689). In the terminal workspace a
 * picture is read in the Files panel's pop-out instead, and that journey is
 * in terminal-links.spec.ts. The image's real pixels and a real pointer drag
 * through pointer capture need a browser, which is why these are here.
 */

const TEXT_PATH = '/tmp/notes.txt'
/** A 3 by 2 PNG, red. */
const PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAMAAAACCAIAAAASFvFNAAAAEElEQVR4nGM4IScHQQxwFgBBAAYZPEVBlgAAAABJRU5ErkJggg==', 'base64')

function seededState() {
  return {
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [{ id: 'terminal1-window-0', boundSessions: ['main'], activeSession: 'main', colorIndex: 0 }],
      },
      terminal2: { windowCount: 1, windows: [] },
      terminal3: { windowCount: 1, windows: [] },
    },
    sidebarCollapsed: false,
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 1000 },
  }
}

/** Files that answer: /tmp lists both files, the PNG is bytes, the text is text. */
async function mockFiles(page: Page) {
  await page.route('**/api/files/raw/**', async route => {
    const png = new URL(route.request().url()).pathname.endsWith('.png')
    await route.fulfill(png
      ? { status: 200, contentType: 'image/png', body: PNG }
      : { status: 200, contentType: 'text/plain', body: 'mock file content' })
  })
  await page.route('**/api/files/diff*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ path: TEXT_PATH, repository: '', diff: '', truncated: false }),
  }))
  await page.route(/\/api\/files\/resources\/?$/, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        isDir: true,
        items: [
          { name: 'shot.png', size: PNG.length, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'image/png' },
          { name: 'other.png', size: PNG.length, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'image/png' },
        ],
      }),
    })
  })
  await page.route(/\/api\/files\/resources\/tmp\/?$/, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        isDir: true,
        items: [
          { name: 'shot.png', size: PNG.length, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'image/png' },
          { name: 'notes.txt', size: 17, modified: '2026-09-03T00:00:00Z', isDir: false, type: 'text/plain' },
        ],
      }),
    })
  })
}

test.describe('the image glance', () => {
  test('never upscales the picture, and closes on Escape and on a press outside', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')

    const glance = await openGlanceFromFilesTab(page)
    await expect(glance.locator('.image-glance-path')).toHaveAttribute('title', '/shot.png')
    await expect(glance.locator('.image-glance-size')).toHaveText('3 × 2')
    // Never upscaled: three pixels wide is three pixels wide.
    await expect.poll(async () => (await glance.locator('img').boundingBox())?.width).toBe(3)

    await page.keyboard.press('Escape')
    await expect(glance).toHaveCount(0)

    await page.getByTestId('file-viewer-scroll').getByRole('button', { name: 'shot.png' }).click()
    await expect(glance).toBeVisible()
    const view = (await page.getByTestId('file-viewer-scroll').boundingBox())!
    await page.mouse.click(view.x + 8, view.y + view.height - 8)
    await expect(glance).toHaveCount(0)
  })

  /**
   * The Files tab's own viewer is where a picture still opens the centred
   * glance: the terminal workspace reads a picture in the panel's pop-out,
   * and the tab has no panel to hang one off.
   */
  async function openGlanceFromFilesTab(page: Page, name = 'shot.png', alreadyOpen = false) {
    await page.click('.tab:has-text("Files")')
    // A reload brings the tab back on the file the operator left open.
    if (!alreadyOpen) await page.click(`.fb-row:has-text("${name}")`)
    await page.getByTestId('file-viewer-scroll').getByRole('button', { name }).click()
    const glance = page.locator('.image-glance')
    await expect(glance).toBeVisible()
    return glance
  }

  test('opens from the picture in the Files tab', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')

    const glance = await openGlanceFromFilesTab(page)
    await expect(glance.locator('.image-glance-size')).toHaveText('3 × 2')

    await glance.getByRole('button', { name: 'Close' }).click()
    await expect(glance).toHaveCount(0)
  })

  test('opens at the size a corner was dragged to, and again after a reload', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')

    const openGlance = (alreadyOpen = false) => openGlanceFromFilesTab(page, 'shot.png', alreadyOpen)

    const glance = await openGlance()
    // The picture is three pixels wide, so the glance opens tiny: its own
    // size, not a minimum.
    const opened = (await glance.boundingBox())!
    expect(opened.width).toBeLessThan(100)

    // Drag the bottom-right corner out. The window is centred, so the corner
    // follows the pointer while the window grows around its middle.
    await page.mouse.move(opened.x + opened.width - 2, opened.y + opened.height - 2)
    await page.mouse.down()
    await page.mouse.move(opened.x + opened.width + 200, opened.y + opened.height + 150, { steps: 10 })
    await page.mouse.up()

    const dragged = (await glance.boundingBox())!
    expect(dragged.width).toBeGreaterThan(opened.width + 300)
    expect(dragged.height).toBeGreaterThan(opened.height + 200)

    await page.reload()
    const reopened = await openGlance(true)
    const after = (await reopened.boundingBox())!
    expect(Math.round(after.width)).toBe(Math.round(dragged.width))
    expect(Math.round(after.height)).toBe(Math.round(dragged.height))

    // The word in the header gives the picture the say back.
    await reopened.getByRole('button', { name: 'Reset size' }).click()
    await expect.poll(async () => (await reopened.boundingBox())!.width).toBeLessThan(100)
  })

  /**
   * The level is the operator's, not the picture's (bead: chrote-4689): a
   * step taken on one picture is the level the next picture opens at. Real
   * image pixels are what makes that visible, which is why it is here.
   */
  test('carries the stepped zoom level to the next picture', async ({ page }) => {
    await mockApiRoutes(page)
    await mockFiles(page)
    await page.addInitScript(state => {
      localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
    }, seededState())
    await page.goto('/')

    const glance = await openGlanceFromFilesTab(page)
    // Three pixels wide inside a window that fits it: drawn at 1:1.
    await expect(glance.locator('.image-glance-zoom')).toHaveText('100%')

    await glance.getByRole('button', { name: 'Zoom in' }).click()
    await expect(glance.locator('.image-glance-zoom')).toHaveText('125%')
    await expect.poll(async () => (await glance.locator('img').boundingBox())?.width).toBe(4)

    await glance.getByRole('button', { name: 'Close' }).click()
    await expect(glance).toHaveCount(0)

    // Close the open file to get the listing back, then the other picture.
    await page.locator('.fb-editor-tab-close').first().click()
    const second = await openGlanceFromFilesTab(page, 'other.png')
    await expect(second.locator('.image-glance-path')).toHaveAttribute('title', '/other.png')
    await expect(second.locator('.image-glance-zoom')).toHaveText('125%')
    await expect.poll(async () => (await second.locator('img').boundingBox())?.width).toBe(4)
  })
})
