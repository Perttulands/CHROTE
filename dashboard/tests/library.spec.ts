/**
 * Journey 7, keep the library (beads: chrote-5grx.17, chrote-5grx.55).
 *
 * One pass through the surface the browser is the point of: land on the map,
 * take a page off a shelf, read it with its neighbours above it, turn the map
 * over and back, open a neighbour from it, and ask the Librarian from the
 * front desk. Everything narrower — a date, a title, where a label sits — is
 * a unit test.
 */

import { test, expect, type Page } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockLibraryApiRoutes } from './mock-api'

/** The Librarian's own session, answering a paste the way tmux would. */
async function mockLibrarianSend(page: Page, sends: string[]) {
  await page.route('**/api/tmux/sessions/*/send', async route => {
    const session = decodeURIComponent(new URL(route.request().url()).pathname.split('/')[4])
    const body = route.request().postData() ?? ''
    const text = /name="text"\r?\n\r?\n([\s\S]*?)\r?\n--/.exec(body)?.[1] ?? ''
    // A multipart text field carries CRLF; the message the operator typed had
    // one newline, so the record is normalised before it is compared.
    sends.push(text.replace(/\r\n/g, '\n'))
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
        submissionRequested: true,
        submitKeyDispatched: true,
        bufferCleaned: true,
        targetVerified: true,
        deliveryConfirmed: true,
        retryable: false,
        warning: '',
      }),
    })
  })
}

/** A page on the map, by the name it carries. */
function node(page: Page, title: string) {
  return page.locator(`.library-map [role="button"][aria-label="${title}"]`)
}

test.describe('The Library', () => {
  test('lands on the map, reads a page with its neighbours, turns the map over, and asks the desk', async ({ page }) => {
    const sends: string[] = []
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page)
    await mockLibrarianSend(page, sends)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await page.click('.tab:has-text("Library")')
    await page.waitForSelector('.library-view')

    // The library lands on the map: every shelf labelled, every link counted.
    await expect(page.locator('.library-map-count')).toHaveText('3 pages · 2 shelves · 2 links · 1 shared tag')
    await expect(page.locator('.library-map-cluster', { hasText: 'preferences · 2' })).toBeVisible()

    // Pointing at a page lights what it touches; the lit dot is drawn by
    // class, and the class is the only thing to read off an SVG.
    await node(page, 'Test isolation').hover()
    await expect(node(page, 'Workflow Preferences')).toHaveClass(/hot/)
    await expect(node(page, 'Tool Preferences')).not.toHaveClass(/hot/)

    // A shelf, then a page off it.
    await page.click('.library-left .library-shelf:has-text("preferences")')
    await page.click('.library-result:has-text("Workflow Preferences")')

    await expect(page.locator('.library-page-title-row h1')).toHaveText('Workflow Preferences')
    await expect(page.locator('.library-body')).toContainText('Prefer small, verifiable changes.')
    await expect(page.locator('.library-page-meta')).toContainText('preferences/workflow.md')
    await expect(page.locator('.library-history-message')).toHaveText('Record a workflow preference')
    await expect(page.locator('.library-linked-from')).toContainText('Test isolation')

    // The strip above the page holds its neighbours, and one of them opens.
    const strip = page.locator('.library-strip')
    await expect(strip).toContainText('Near this page')
    await expect(strip.locator('[aria-label="Tool Preferences"]')).toBeVisible()

    // Alt+R turns the map over with the page still on the table, and back.
    await page.keyboard.press('Alt+r')
    await expect(page.locator('.library-map-frame')).toBeVisible()
    await expect(page.locator('.library-page')).toHaveCount(0)
    await expect(node(page, 'Workflow Preferences')).toHaveClass(/hot/)

    await node(page, 'Tool Preferences').click()
    await expect(page.locator('.library-page-title-row h1')).toHaveText('Tool Preferences')

    await page.keyboard.press('Alt+r')
    await expect(page.locator('.library-map-frame')).toBeVisible()
    await page.keyboard.press('Alt+r')
    await expect(page.locator('.library-page-title-row h1')).toHaveText('Tool Preferences')

    // The front desk sends the question with the page as its first line, and
    // the status line is the receipt.
    await page.fill('.desk-ask', 'What else says this?')
    await page.press('.desk-ask', 'Enter')

    await expect.poll(() => sends).toEqual(['library preferences/tools.md\nWhat else says this?'])
    await expect(page.locator('.status-line')).toContainText('Asked hq-deacon')
    await expect(page.locator('.desk-ask')).toHaveValue('')
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
    await page.waitForSelector('.library-proposal')

    const box = async (selector: string) => {
      const found = await page.locator(selector).boundingBox()
      if (!found) throw new Error(`${selector} has no box`)
      return found
    }
    const shelves = await box('.library-shelves')
    const arrivals = await box('.library-arrivals')
    const proposals = await box('.library-proposals')

    expect(shelves.y + shelves.height).toBeLessThanOrEqual(arrivals.y + 0.5)
    expect(arrivals.y + arrivals.height).toBeLessThanOrEqual(proposals.y + 0.5)
    expect(proposals.height).toBeGreaterThan(40)

    for (const row of await page.locator('.library-shelf').all()) {
      const rowBox = await row.boundingBox()
      expect(rowBox).not.toBeNull()
      expect(rowBox!.y).toBeGreaterThanOrEqual(shelves.y - 0.5)
      expect(rowBox!.y + rowBox!.height).toBeLessThanOrEqual(shelves.y + shelves.height + 0.5)
    }

    const arrivalsScroll = page.locator('.library-arrivals .library-scroll')
    expect(await arrivalsScroll.evaluate(element => element.scrollHeight > element.clientHeight)).toBe(true)
    await expect(page.locator('.library-arrival').last()).not.toBeInViewport()
  })

  test('says so when the host has no library', async ({ page }) => {
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page, {
      shelves: { root: '', shelves: [], librarianSession: '', beadsProject: '' },
    })
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await page.click('.tab:has-text("Library")')

    await expect(page.locator('.library-view')).toContainText('No library is configured')
  })
})
