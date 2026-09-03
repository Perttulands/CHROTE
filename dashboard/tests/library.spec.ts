/**
 * Journey 7, keep the library (bead: chrote-5grx.17).
 *
 * One pass through the surface the browser is the point of: step into the
 * library, take a page off a shelf, read it, and ask the Librarian about it
 * from the front desk. Everything narrower — a date, a title, a state word —
 * is a unit test.
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

test.describe('The Library', () => {
  test('steps into the library, opens a page from a shelf, and asks the desk', async ({ page }) => {
    const sends: string[] = []
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockLibraryApiRoutes(page)
    await mockLibrarianSend(page, sends)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await page.click('.tab:has-text("Library")')
    await page.waitForSelector('.library-view')

    // The reading room opens on the shelves themselves.
    await expect(page.locator('.library-page-meta')).toContainText('3 pages on 2 shelves')
    await expect(page.locator('.library-card', { hasText: 'preferences' })).toBeVisible()

    // A shelf, then a page off it.
    await page.click('.library-left .library-shelf:has-text("preferences")')
    await page.click('.library-result:has-text("Workflow Preferences")')

    await expect(page.locator('.library-page-title-row h1')).toHaveText('Workflow Preferences')
    await expect(page.locator('.library-body')).toContainText('Prefer small, verifiable changes.')
    await expect(page.locator('.library-page-meta')).toContainText('preferences/workflow.md')
    await expect(page.locator('.library-history-message')).toHaveText('Record a workflow preference')
    await expect(page.locator('.library-right')).toContainText('On preferences')

    // The front desk sends the question with the page as its first line, and
    // the status line is the receipt.
    await page.fill('.desk-ask', 'What else says this?')
    await page.press('.desk-ask', 'Enter')

    await expect.poll(() => sends).toEqual(['library preferences/workflow.md\nWhat else says this?'])
    await expect(page.locator('.status-line')).toContainText('Asked hq-deacon')
    await expect(page.locator('.desk-ask')).toHaveValue('')
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
