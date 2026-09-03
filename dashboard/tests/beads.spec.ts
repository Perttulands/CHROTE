/**
 * The Beads tab and the Bead card (bead: chrote-5grx.15).
 */

import { test, expect, allowBrowserConsoleMessage } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockBeadsApiError } from './mock-api'

async function openBeadsTab(page: import('@playwright/test').Page) {
  await page.click('.tab:has-text("Beads")')
  await page.waitForSelector('.beads-view')
}

test.describe('Beads', () => {
  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test('opens on the map of every configured store', async ({ page }) => {
    await openBeadsTab(page)

    await expect(page.locator('.beads-rail-item').first()).toHaveText('All')
    await expect(page.locator('.beads-rail-item.active')).toHaveText('All')
    await expect(page.locator('.bead-row', { hasText: 'One interaction language' })).toBeVisible()
    await expect(page.locator('.bead-map-acceptance')).toContainText('Every surface reads the same way')
    await expect(page.locator('.bead-row-blocked')).toContainText('blocked by test-ep1.2')
  })

  test('folds an epic and narrows by search', async ({ page }) => {
    await openBeadsTab(page)

    await page.click('.bead-row:has-text("One interaction language") .bead-row-glyph')
    await expect(page.locator('.bead-row', { hasText: 'Fix login bug' })).toHaveCount(0)

    await page.click('.bead-row:has-text("One interaction language") .bead-row-glyph')
    await page.fill('.beads-search', 'dark mode')
    await expect(page.locator('.bead-row', { hasText: 'Add dark mode' })).toBeVisible()
    await expect(page.locator('.bead-row', { hasText: 'Fix login bug' })).toHaveCount(0)
  })

  test('splits ready from in progress, and lists what has gone stale', async ({ page }) => {
    await openBeadsTab(page)

    await page.click('.beads-view-tab:has-text("Ready and in progress")')
    const ready = page.locator('.beads-column').first()
    const inProgress = page.locator('.beads-column').nth(1)
    await expect(ready).toContainText('Fix login bug')
    await expect(inProgress).toContainText('Add dark mode')
    await expect(ready).not.toContainText('Blocked by external API')

    await page.click('.beads-view-tab:has-text("Stale")')
    await expect(page.locator('.bead-row')).toHaveCount(1)
    await expect(page.locator('.bead-row')).toContainText('Blocked by external API')
    await expect(page.locator('.bead-row-age')).toContainText('days')
  })

  test('opens the card from a row and hands the Bead to a session', async ({ page }) => {
    await openBeadsTab(page)

    await page.click('.bead-row:has-text("Fix login bug") .bead-row-open')

    const card = page.locator('.sheet-right[aria-label="Bead test-ep1.1"]')
    await expect(card).toBeVisible()
    await expect(card.locator('.bead-card-title')).toHaveText('Fix login bug')
    await expect(card).toContainText('A login survives a reload.')
    await expect(card.locator('.bead-card-fields')).toContainText('test-ep1')

    // Copy id confirms as a toast in the bottom-centre slot, and the status
    // line keeps the same event as the record.
    await card.getByRole('button', { name: 'Copy id' }).click()
    await expect(page.locator('.toast')).toHaveText('Copied test-ep1.1')
    await expect(page.locator('.status-line')).toContainText('Copied test-ep1.1')

    await card.getByRole('button', { name: 'Send' }).click()
    await expect(page.locator('.send-drawer-reference')).toHaveText('bead test-ep1.1: Fix login bug')
  })

  test('follows an id inside the card and comes back', async ({ page }) => {
    await openBeadsTab(page)
    await page.click('.bead-row:has-text("Fix login bug") .bead-row-open')

    await page.click('.bead-card-section .chrote-markdown-token')

    await expect(page.locator('.bead-card-id')).toHaveText('test-ep1.2')
    await page.click('.bead-card-action:has-text("Back")')
    await expect(page.locator('.bead-card-id')).toHaveText('test-ep1.1')
  })

  test('says what refused rather than showing a blank tab', async ({ page }) => {
    allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 503')
    await mockBeadsApiError(page)

    await openBeadsTab(page)

    await expect(page.locator('.beads-error')).toContainText('bd command not found')
  })
})
