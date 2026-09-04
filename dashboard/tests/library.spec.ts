/**
 * Journey 7, keep the library (beads: chrote-5grx.17, chrote-5grx.55,
 * chrote-5grx.75, chrote-5grx.76, chrote-5grx.77).
 *
 * One pass through the surface the browser is the point of: land on the map,
 * read a long name in full under the pointer, narrow the recency window, take
 * the map in and put it back, open a shelf in the rail and point at one of its
 * pages to bring the map to it, dive into a page, travel to a neighbour, walk
 * the trail back, hand the page to the Librarian in the column at the right,
 * and close the dive with the map still standing. Everything narrower — a date, a
 * title, where a label sits — is a unit test.
 */

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

test.describe('The Library', () => {
  test('lands on the map, dives into a page, travels to a neighbour and back, and hands it to the Librarian', async ({ page }) => {
    const sends: { text: string; submit: string }[] = []
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page)
    await mockLibrarianSend(page, sends)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await page.click('.tab:has-text("Library")')
    await page.waitForSelector('.library-view')

    // The library lands on the map: every shelf labelled, every link counted.
    await expect(page.locator('.library-map-count')).toHaveText('4 pages · 2 shelves · 2 links · 1 shared tag')
    await expect(page.locator('.library-map-cluster', { hasText: 'preferences · 2' })).toBeVisible()

    // Pointing at a page lights what it touches; the lit dot is drawn by
    // class, and the class is the only thing to read off an SVG.
    await node(page, 'Test isolation').hover()
    await expect(node(page, 'Workflow Preferences')).toHaveClass(/hot/)
    await expect(node(page, 'Tool Preferences')).not.toHaveClass(/hot/)

    // A name too long for the map's labels is still readable in full under
    // the pointer, where the label beside the dot only shortens it.
    await node(page, LONG_TITLE).hover()
    await expect(page.locator('[data-ui="library.map.hover"]')).toHaveText(LONG_TITLE)

    // The recency window leaves the corpus in place and steps back from what
    // has not moved lately; All brings it forward again.
    await page.click('.library-map-window:has-text("Week")')
    await expect(page.locator('.library-map-legend')).toHaveText('Dimmed: not changed in the last 7 days')
    await expect(node(page, LONG_TITLE)).toHaveClass(/stale/)
    await expect(node(page, 'Test isolation')).not.toHaveClass(/stale/)
    await page.click('.library-map-window:has-text("All")')
    await expect(node(page, LONG_TITLE)).not.toHaveClass(/stale/)

    // The map moves: the wheel takes it in, and the way back is offered only
    // while there is one.
    const reset = page.locator('[data-ui="library.map.reset"]')
    await expect(reset).toHaveCount(0)
    await page.locator('.library-map canvas').hover({ position: { x: 300, y: 200 } })
    await page.mouse.wheel(0, -400)
    await expect(reset).toBeVisible()
    await reset.click()
    await expect(reset).toHaveCount(0)

    // The rail works the map rather than replacing it: a shelf draws open
    // where it stands, and pointing at one of its pages takes the map to that
    // page and lights it.
    await page.click('.library-shelf:has-text("preferences")')
    const row = page.locator('.library-shelves .library-row', { hasText: 'Workflow Preferences' })
    await expect(row).toBeVisible()
    await expect(page.locator('.library-map')).toBeVisible()

    await row.locator('.library-row-head').hover()
    await expect(node(page, 'Workflow Preferences')).toHaveClass(/hot/)
    await expect(reset).toBeVisible()
    await reset.click()

    // Clicking the row opens it on what the page is, in place.
    await row.locator('.library-row-head').click()
    await expect(row.locator('.library-row-meta')).toContainText('200 words')

    // A dot on the map dives into its page: the page opens beside the map,
    // the map stays and takes the page it was asked for.
    await node(page, 'Workflow Preferences').click()

    const dive = page.locator('.library-dive')
    await expect(dive.locator('.library-page-title-row h1')).toHaveText('Workflow Preferences')
    await expect(dive.locator('.library-body')).toContainText('Prefer small, verifiable changes.')
    await expect(dive.locator('.library-history-message')).toHaveText('Record a workflow preference')
    await expect(page.locator('.library-map')).toBeVisible()
    await expect(page.locator('[data-ui="library.map.reset"]')).toBeVisible()

    // A dive is a reading, so it takes the map near enough to read: the page it
    // opened carries a card beside its dot saying what the page is, not only
    // what it is called.
    const card = page.locator('[data-ui="library.map.hover"]')
    await expect(card).toContainText('Workflow Preferences')
    await expect(card).toContainText('preferences ·')
    await expect(card).toContainText('200 words')

    // The trail starts at the page it started at.
    await expect(dive.locator('.library-trail-step')).toHaveText(['Workflow Preferences'])

    // A neighbour travels, and the trail grows behind the reader.
    await dive.locator('.library-links', { hasText: 'Neighbours' })
      .getByRole('button', { name: 'Tool Preferences' }).click()
    await expect(dive.locator('.library-page-title-row h1')).toHaveText('Tool Preferences')
    await expect(dive.locator('.library-trail-step')).toHaveText(['Workflow Preferences', 'Tool Preferences'])
    await expect(node(page, 'Tool Preferences')).toHaveClass(/hot/)

    // The Librarian is live in the column at the right, and Alt+S puts the
    // page into its prompt on a line of its own, for the operator to finish.
    const column = page.getByRole('complementary', { name: 'The Librarian' })
    await expect(column.locator('.resident-header')).toContainText('hq-deacon')
    await expect(column.locator('.xterm-rows')).toContainText('mock terminal hq-deacon')
    await page.keyboard.press('Alt+s')

    await expect.poll(() => sends).toEqual([{ text: 'library preferences/tools.md\n', submit: 'false' }])
    await expect(page.locator('.status-line')).toContainText("Pasted to 'hq-deacon'")

    // A step of the trail goes back, and cuts the trail to where it went.
    await dive.locator('.library-trail-step', { hasText: 'Workflow Preferences' }).click()
    await expect(dive.locator('.library-page-title-row h1')).toHaveText('Workflow Preferences')
    await expect(dive.locator('.library-trail-step')).toHaveText(['Workflow Preferences'])

    // Escape ends the dive; the map is still there, and still where it was.
    await page.keyboard.press('Escape')
    await expect(dive).toHaveCount(0)
    await expect(node(page, 'Workflow Preferences')).toBeVisible()
  })

  // Real layout is the only way to see one section painting over another;
  // the rule these boxes prove is that every section takes a fixed or a
  // scrolling extent (chrote-5grx.35).
  test('keeps every rail section inside its own extent at a short window', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 700 })
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await page.click('.tab:has-text("Library")')
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

  test('says so when the host has no library', async ({ page }) => {
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page, {
      shelves: { root: '', shelves: [], librarianSession: '' },
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await page.click('.tab:has-text("Library")')

    await expect(page.locator('.library-view')).toContainText('No library is configured')
  })
})
