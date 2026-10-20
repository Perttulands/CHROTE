/** Brain exploration, reading, Librarian handoff and browser geometry. */

import { test, expect, type Page } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockLibraryApiRoutes } from './mock-api'

/** The Librarian's own session, accepting a paste the way tmux would. */
async function mockLibrarianSend(page: Page, sends: { text: string; submit: string }[]) {
  await page.route('**/api/tmux/sessions/*/send', async route => {
    const session = decodeURIComponent(new URL(route.request().url()).pathname.split('/')[4])
    const body = route.request().postData() ?? ''
    const text = /name="text"\r?\n\r?\n([\s\S]*?)\r?\n--/.exec(body)?.[1] ?? ''
    const submit = /name="submit"\r?\n\r?\n([\s\S]*?)\r?\n--/.exec(body)?.[1] ?? ''
    // A multipart text field carries CRLF; the line the column pasted ends in
    // one newline, so the record is normalised before it is compared.
    sends.push({ text: text.replace(/\r\n/g, '\n'), submit })
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        session,
        sessionId: '$1',
        pane: '%1',
        panePid: '4242',
        serverPid: '9001',
        unixUser: '',
        transport: 'pasted',
        submissionRequested: submit === 'true',
        submitKeyDispatched: submit === 'true',
        bufferCleaned: true,
        targetVerified: true,
        deliveryConfirmed: true,
        retryable: false,
        warning: '',
      }),
    })
  })
}

/** The fixture's page whose name is past the measure a label is drawn at. */
const LONG_TITLE = 'A note whose name runs well past the measure a label is drawn at'

/**
 * A page on the map, by the name it carries. The drawing is a canvas; what a
 * page has of its own is the element over it that the keyboard reaches it by,
 * and that element is what a pointer lands on too.
 */
function node(page: Page, title: string) {
  return page.locator(`.library-map button[aria-label="${title}"]`)
}

test.describe('The Brain', () => {
  test('explores subjects, reads a page and its neighbour, and hands it to the Librarian', async ({ page }) => {
    const sends: { text: string; submit: string }[] = []
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page)
    await mockLibrarianSend(page, sends)
    await page.goto('/')
    await page.getByRole('button', { name: 'Brain', exact: true }).click()

    await expect(page.locator('.library-map-count')).toHaveText('4 pages · 2 subjects · 2 links')
    const preferences = page.getByRole('button', { name: 'preferences, 2 pages' })
    await expect(preferences).toHaveAttribute('aria-expanded', 'false')
    await preferences.focus()
    await page.keyboard.press('Enter')
    await expect(preferences).toHaveAttribute('aria-expanded', 'true')
    await expect(node(page, 'Workflow Preferences')).toBeVisible()
    await expect(node(page, 'Test isolation')).toHaveCount(0)

    // Selecting a page shows its references before opening the reader.
    await node(page, 'Workflow Preferences').click()
    await expect(page.locator('.brain-page-selection')).toContainText('Workflow Preferences')
    await expect(page.locator('.brain-external').getByRole('button', { name: /^Test isolation/ })).toBeVisible()
    await expect(page.locator('.library-dive')).toHaveCount(0)
    await page.getByRole('button', { name: 'Open page', exact: true }).click()
    const dive = page.locator('.library-dive')
    await expect(dive.locator('.library-page-title-row h1')).toHaveText('Workflow Preferences')
    await expect(dive.locator('.library-body')).toContainText('Prefer small, verifiable changes.')
    await expect(dive.locator('.library-history-message')).toHaveText('Record a workflow preference')
    await expect(dive.locator('.library-trail-step')).toHaveText(['Workflow Preferences'])
    await expect(page.locator('.library-map')).toBeVisible()

    await dive.locator('.library-links', { hasText: 'Neighbours' })
      .getByRole('button', { name: 'Tool Preferences' }).click()
    await expect(dive.locator('.library-page-title-row h1')).toHaveText('Tool Preferences')
    await expect(dive.locator('.library-trail-step')).toHaveText(['Workflow Preferences', 'Tool Preferences'])
    const column = page.getByRole('complementary', { name: 'The Librarian' })
    await expect(column.locator('.resident-header')).toContainText('hq-deacon')
    await expect(column.locator('.xterm-rows')).toContainText('mock terminal hq-deacon')
    await page.keyboard.press('Alt+s')
    await expect.poll(() => sends).toEqual([{ text: 'library preferences/tools.md\n', submit: 'false' }])

    await dive.locator('.library-trail-step', { hasText: 'Workflow Preferences' }).click()
    await expect(dive.locator('.library-page-title-row h1')).toHaveText('Workflow Preferences')
    await expect(dive.locator('.library-trail-step')).toHaveText(['Workflow Preferences'])
    await page.keyboard.press('Escape')
    await expect(dive).toHaveCount(0)
    await page.getByRole('button', { name: 'Overview', exact: true }).click()
    await expect(preferences).toHaveAttribute('aria-expanded', 'false')

    // Expanding an older subject preserves full hover labels, time and zoom.
    await page.getByRole('button', { name: 'knowledge, 2 pages' }).click()
    await node(page, LONG_TITLE).hover()
    await expect(page.locator('[data-ui="library.map.hover"]')).toContainText(LONG_TITLE)
    const scrubber = page.locator('[data-ui="library.map.scrubber"]')
    await scrubber.fill('670')
    await expect(page.locator('.library-map-legend')).toHaveText('The corpus 7 days ago')
    await expect(node(page, 'Test isolation')).toHaveCount(0)
    await expect(node(page, LONG_TITLE)).toBeVisible()
    await scrubber.fill('1000')
    await expect(node(page, 'Test isolation')).toBeVisible()
    const reset = page.locator('[data-ui="library.map.reset"]')
    await page.getByRole('img', { name: 'The map' }).hover()
    await page.mouse.wheel(0, -400)
    await expect(reset).toBeVisible()
    await reset.click()
    await expect(reset).toHaveCount(0)
  })

  // Layout and canvas hit targets require a real browser at a narrow width.
  test('keeps subject selection and reader controls reachable in a narrow window', async ({ page }) => {
    await page.setViewportSize({ width: 760, height: 700 })
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page)
    await page.goto('/')
    await page.getByRole('button', { name: '☰', exact: true }).click()
    await page.getByRole('button', { name: 'Brain', exact: true }).click()
    await page.getByRole('button', { name: 'preferences, 2 pages' }).click()
    await node(page, 'Workflow Preferences').click()
    const open = page.getByRole('button', { name: 'Open page', exact: true })
    await expect(open).toBeInViewport()
    const box = await open.boundingBox()
    expect(box).not.toBeNull()
    expect(box!.x).toBeGreaterThanOrEqual(0)
    expect(box!.x + box!.width).toBeLessThanOrEqual(760)
    await open.click()
    const dive = page.locator('.library-dive')
    await expect(dive.locator('.library-body')).toContainText('Prefer small, verifiable changes.')
    await expect(dive.getByRole('button', { name: /^Close/ })).toBeInViewport()
    await page.keyboard.press('Escape')
    await expect(dive).toHaveCount(0)
    await expect(page.getByRole('button', { name: 'Overview', exact: true })).toBeInViewport()
  })

  test('keeps collection visibility across a reload', async ({ page }) => {
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page)
    await page.goto('/')
    await page.getByRole('button', { name: 'Brain', exact: true }).click()
    await expect(page.locator('.library-map-count')).toHaveText('4 pages · 2 subjects · 2 links')
    await page.locator('.library-shelf-row', { hasText: 'preferences' })
      .getByRole('button', { name: 'Hide preferences on the map' }).click()
    await expect(page.locator('.library-map-count')).toHaveText('2 pages · 1 subject · 0 links')
    await expect(page.getByRole('button', { name: 'preferences, 2 pages' })).toHaveCount(0)
    await page.reload()
    await page.getByRole('button', { name: 'Brain', exact: true }).click()
    await expect(page.locator('.library-map-count')).toHaveText('2 pages · 1 subject · 0 links')
    await page.locator('.library-shelf-row', { hasText: 'preferences' })
      .getByRole('button', { name: 'Show preferences on the map' }).click()
    await expect(page.locator('.library-map-count')).toHaveText('4 pages · 2 subjects · 2 links')
    await expect(page.getByRole('button', { name: 'preferences, 2 pages' })).toBeVisible()
  })

  test('keeps every rail section inside its own extent at a short window', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 700 })
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await page.getByRole('button', { name: 'Brain', exact: true }).click()
    await page.waitForSelector('.library-arrivals .library-row')

    const box = async (selector: string) => {
      const found = await page.locator(selector).boundingBox()
      if (!found) throw new Error(`${selector} has no box`)
      return found
    }
    const shelves = await box('.library-shelves')
    const arrivals = await box('.library-arrivals')

    expect(shelves.y + shelves.height).toBeLessThanOrEqual(arrivals.y + 0.5)
    expect(shelves.height).toBeGreaterThan(40)
    expect(arrivals.height).toBeGreaterThan(40)

    // A shelf drawn open lists its pages inside the shelves section, however
    // long the listing is: the section scrolls rather than growing over its
    // neighbour.
    await page.click('.library-shelf:has-text("knowledge")')
    await expect(page.locator('.library-shelf-pages .library-row').first()).toBeVisible()
    const opened = await box('.library-shelves')
    for (const row of await page.locator('.library-shelf, .library-shelf-pages .library-row').all()) {
      const rowBox = await row.boundingBox()
      if (!rowBox) continue
      expect(rowBox.y).toBeGreaterThanOrEqual(opened.y - 0.5)
      expect(rowBox.y + rowBox.height).toBeLessThanOrEqual(opened.y + opened.height + 0.5)
    }

    const arrivalsScroll = page.locator('.library-arrivals .library-scroll')
    expect(await arrivalsScroll.evaluate(element => element.scrollHeight > element.clientHeight)).toBe(true)
    await expect(page.locator('.library-arrivals .library-row').last()).not.toBeInViewport()
  })
})
