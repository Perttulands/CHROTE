/**
 * A tab keeps the work the operator left in it (bead: chrote-5grx.73).
 *
 * The browser is the point here. Three real tab switches prove that local
 * selection survives, the front resident alone owns the keyboard, and no
 * view asks the server for the same data again on return.
 */

import { test, expect, type Page } from './fixtures'
import {
  mockApiRoutes,
  mockResidentsApiRoute,
} from './mock-api'

function multipartField(body: string, name: string): string {
  const match = new RegExp(`name="${name}"\\r?\\n\\r?\\n([\\s\\S]*?)\\r?\\n--`).exec(body)
  return match ? match[1].replace(/\r\n/g, '\n') : ''
}

async function recordResidentSends(page: Page) {
  const sends: { session: string; text: string }[] = []
  await page.route('**/api/tmux/sessions/*/send', async route => {
    const session = decodeURIComponent(new URL(route.request().url()).pathname.split('/')[4])
    const text = multipartField(route.request().postData() ?? '', 'text')
    sends.push({ session, text })
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
        submissionRequested: false,
        submitKeyDispatched: false,
        bufferCleaned: true,
        targetVerified: true,
        deliveryConfirmed: true,
        retryable: false,
        warning: '',
      }),
    })
  })
  return sends
}

test('returns to Beads, Library and Agents exactly as they were left', async ({ page }) => {
  const reads = new Map<string, number>()
  page.on('request', request => {
    if (request.method() !== 'GET') return
    const url = new URL(request.url())
    if (![
      '/api/workspaces',
      '/api/residents',
      '/api/beads/work',
      '/api/beads/issue',
      '/api/library/shelves',
      '/api/library/changes',
      '/api/library/graph',
      '/api/library/pages',
      '/api/library/page',
      '/api/agent/tender',
      '/api/agent/context',
    ].includes(url.pathname)) return
    const key = `${url.pathname}${url.search}`
    reads.set(key, (reads.get(key) ?? 0) + 1)
  })

  await mockApiRoutes(page)
  await mockResidentsApiRoute(page, [
    { tab: 'library', label: 'Librarian', session: 'hq-deacon', folder: '/corpus', beads: '/code/test-project' },
    { tab: 'agents', label: 'Tender', session: 'hq-mayor', folder: '/code/tender', beads: '/code/test-project' },
    { tab: 'beads', label: 'Clerk', session: 'main', folder: '/code/clerk', beads: '/code/test-project' },
  ])
  const sends = await recordResidentSends(page)

  await page.goto('/')
  await page.waitForSelector('.dashboard')

  await page.getByRole('button', { name: 'Beads', exact: true }).click()
  await page.locator('.bead-row', { hasText: 'Fix login bug' }).getByRole('button').click()
  await expect(page.getByRole('complementary', { name: 'Bead test-ep1.1' })).toContainText('Fix login bug')
  await page.keyboard.press('Alt+Enter')
  await expect(page.getByRole('complementary', { name: 'The Clerk' }).locator('.xterm-helper-textarea')).toBeFocused()
  await page.keyboard.press('Alt+s')
  await expect.poll(() => sends[sends.length - 1]).toEqual({ session: 'main', text: 'bead test-ep1.1: Fix login bug\n' })

  await page.getByRole('button', { name: 'Library', exact: true }).click()
  await page.locator('.library-shelf', { hasText: 'preferences' }).click()
  const shelfRow = page.locator('.library-shelves .library-row', { hasText: 'Workflow Preferences' })
  await shelfRow.locator('.library-row-head').click()
  await shelfRow.getByRole('button', { name: 'Dive' }).click()
  await expect(page.getByRole('heading', { name: 'Workflow Preferences' })).toBeVisible()
  await page.keyboard.press('Alt+Enter')
  await expect(page.getByRole('complementary', { name: 'The Librarian' }).locator('.xterm-helper-textarea')).toBeFocused()
  await page.keyboard.press('Alt+s')
  await expect.poll(() => sends[sends.length - 1]).toEqual({ session: 'hq-deacon', text: 'library preferences/workflow.md\n' })

  await page.getByRole('button', { name: 'Agents', exact: true }).click()
  await page.locator('.agents-view').getByTitle('/code/test-project').click()
  await expect(page.locator('.agents-folder')).toHaveText('/code/test-project')
  await expect(page.locator('.agents-view').getByRole('button', { name: /\/code\/test-project\/CLAUDE\.md/ })).toBeVisible()
  await page.keyboard.press('Alt+Enter')
  await expect(page.getByRole('complementary', { name: 'The Tender' }).locator('.xterm-helper-textarea')).toBeFocused()
  await page.keyboard.press('Alt+s')
  await expect.poll(() => sends[sends.length - 1]).toEqual({ session: 'hq-mayor', text: 'agents /code/test-project claude-code\n' })

  await page.keyboard.press('Alt+k')
  await page.getByRole('textbox', { name: 'Search keybindings' }).fill('Paste into the')
  const residentChord = page.locator('.keys-panel-chord')
  await expect(residentChord).toHaveCount(1)
  await expect(residentChord).toContainText('ALT + S')
  await expect(residentChord).toContainText('Paste into the Tender')
  await page.keyboard.press('Escape')

  const readsBeforeReturn = new Map(reads)

  await page.getByRole('button', { name: 'Beads', exact: true }).click()
  await expect(page.getByRole('complementary', { name: 'Bead test-ep1.1' })).toContainText('Fix login bug')

  await page.getByRole('button', { name: 'Library', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Workflow Preferences' })).toBeVisible()

  await page.getByRole('button', { name: 'Agents', exact: true }).click()
  await expect(page.locator('.agents-folder')).toHaveText('/code/test-project')
  await expect(page.locator('.agents-view').getByRole('button', { name: /\/code\/test-project\/CLAUDE\.md/ })).toBeVisible()

  await page.evaluate(() => new Promise<void>(resolve => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()))
  }))
  expect(reads).toEqual(readsBeforeReturn)
})
