/**
 * The resident's column (bead: chrote-5grx.50): the Clerk lives in the Beads
 * tab, the Bead on the table goes into its prompt on Alt+S, and the column
 * keeps the width it was given. What the browser is the point of here: a live
 * terminal in the column, a chord routed while that terminal has the keyboard,
 * and a drag on real geometry that outlives a reload.
 */

import { test, expect, type Locator, type Page } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockResidentsApiRoute } from './mock-api'

interface SendRecord {
  text: string
  submit: string
}

function field(body: string, name: string): string {
  const match = new RegExp(`name="${name}"\\r?\\n\\r?\\n([\\s\\S]*?)\\r?\\n--`).exec(body)
  // A multipart text field carries CRLF; the line the column pasted ends in
  // one newline, so the record is normalised before it is compared.
  return match ? match[1].replace(/\r\n/g, '\n') : ''
}

/** The resident's own session, accepting what is pasted into it. */
async function mockResidentSend(page: Page, sends: SendRecord[]) {
  await page.route('**/api/tmux/sessions/*/send', async route => {
    const session = decodeURIComponent(new URL(route.request().url()).pathname.split('/')[4])
    const body = route.request().postData() ?? ''
    const submit = field(body, 'submit')
    sends.push({ text: field(body, 'text'), submit })
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

async function openBeadsTab(page: Page) {
  await page.click('.tab:has-text("Beads")')
  await page.waitForSelector('.beads-view')
}

async function box(locator: Locator) {
  const value = await locator.boundingBox()
  if (!value) throw new Error('expected a rendered bounding box')
  return value
}

test.describe('The resident column', () => {
  test('shows the Clerk live, pastes the table into its prompt, and keeps the width it was given', async ({ page }) => {
    const sends: SendRecord[] = []
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockResidentSend(page, sends)
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await openBeadsTab(page)
    const column = page.getByRole('complementary', { name: 'The Clerk' })
    await expect(column.locator('.resident-header')).toContainText('hq-deacon')
    await expect(column.locator('.resident-state')).toHaveText('live')
    await expect(column.locator('.xterm-rows')).toContainText('mock terminal hq-deacon')

    // A Bead on the table, then Alt+S: the reference lands in the Clerk's
    // prompt on a line of its own, nothing is submitted, no drawer opens, and
    // the keyboard is in the Clerk's terminal.
    await page.click('.bead-row:has-text("Fix login bug") .bead-row-open')
    await expect(page.getByRole('complementary', { name: 'Bead test-ep1.1' })).toBeVisible()
    await page.keyboard.press('Alt+s')
    await expect.poll(() => sends).toEqual([{ text: 'bead test-ep1.1: Fix login bug\n', submit: 'false' }])
    await expect(page.getByRole('dialog', { name: 'Send to session' })).toHaveCount(0)
    await expect(column.locator('.xterm-helper-textarea')).toBeFocused()

    // The handle widens the column by what the pointer moved, and the width
    // is the device's: it is there again after a reload.
    const before = await box(column)
    const handle = column.locator('.resident-column-handle')
    const handleBox = await box(handle)
    const grabX = handleBox.x + 2
    await page.mouse.move(grabX, handleBox.y + 200)
    await page.mouse.down()
    await page.mouse.move(grabX - 40, handleBox.y + 200, { steps: 4 })
    await page.mouse.move(grabX - 80, handleBox.y + 200, { steps: 4 })
    await page.mouse.up()
    const widened = Math.round(before.width) + 80
    expect(Math.round((await box(column)).width)).toBe(widened)

    await page.reload()
    await page.waitForSelector('.dashboard')
    await openBeadsTab(page)
    const reopened = page.getByRole('complementary', { name: 'The Clerk' })
    await expect(reopened.locator('.resident-state')).toHaveText('live')
    expect(Math.round((await box(reopened)).width)).toBe(widened)
  })

  test('offers Launch with the Clerk folder when its session is absent', async ({ page }) => {
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await mockResidentsApiRoute(page, [
      { tab: 'beads', label: 'Clerk', session: 'clerk', folder: '/code/clerk', beads: '' },
    ])
    await page.goto('/')
    await page.waitForSelector('.dashboard')

    await openBeadsTab(page)
    const column = page.getByRole('complementary', { name: 'The Clerk' })
    await expect(column.locator('.resident-state')).toHaveText('not running')

    await column.getByRole('button', { name: 'Launch' }).click()
    const launcher = column.locator('.launcher')
    await expect(launcher.getByLabel('Session name')).toHaveValue('clerk')
    await expect(launcher.getByRole('button', { name: /in clerk$/ })).toBeVisible()
  })
})
