/**
 * The Beads tab, the Bead card on the table, and the drawer over it
 * (beads: chrote-5grx.15, chrote-5grx.47).
 */

import { test, expect, allowBrowserConsoleMessage, type Locator, type Page } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockBeadsApiError } from './mock-api'

async function openBeadsTab(page: Page) {
  await page.click('.tab:has-text("Beads")')
  await page.waitForSelector('.beads-view')
}

async function box(locator: Locator) {
  const value = await locator.boundingBox()
  if (!value) throw new Error('expected a rendered bounding box')
  return value
}

test.describe('Beads', () => {
  // The tab holds a rail, the map, the table and the Clerk's column side by
  // side; at the default 1280px the squeeze narrows the table before the
  // widths this journey asserts, so it runs at a desktop width.
  test.use({ viewport: { width: 1600, height: 900 } })

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

  test('folds an epic from its title and narrows by search', async ({ page }) => {
    await openBeadsTab(page)

    const epic = page.locator('.bead-row', { hasText: 'One interaction language' })
    await expect(epic.locator('.bead-row-fold')).toHaveText('▾4')
    await epic.locator('.bead-row-title').click()
    await expect(page.locator('.bead-row', { hasText: 'Fix login bug' })).toHaveCount(0)
    await expect(epic.locator('.bead-row-fold')).toHaveText('▸4')
    // The same click put the epic on the table.
    await expect(page.getByRole('complementary', { name: 'Bead test-ep1' })).toBeVisible()

    await page.fill('.beads-search', 'dark mode')
    await expect(page.locator('.bead-row', { hasText: 'Add dark mode' })).toBeVisible()
    await expect(page.locator('.bead-row', { hasText: 'Fix login bug' })).toHaveCount(0)
    await expect(epic.locator('.bead-row-fold')).toHaveText('▾1')
  })

  // The row's menu from a real right-click, and the copy it runs landing on
  // the clipboard from a menu click: the toast is the receipt.
  test('copies a Bead\'s id from its row\'s menu', async ({ page }) => {
    await openBeadsTab(page)

    await page.locator('.bead-row', { hasText: 'Fix login bug' }).click({ button: 'right' })
    const menu = page.getByRole('menu', { name: 'Actions for test-ep1.1' })
    await menu.getByRole('menuitem', { name: 'Copy id', exact: true }).click()

    await expect(page.locator('.toast')).toHaveText('Copied test-ep1.1')
    await expect(menu).toHaveCount(0)
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

  // The right edge, end to end: a Bead goes on the table from the map, the
  // drawer lies over the table's column and gives it back on Escape, the same
  // Bead is in a column beside the tiles on a terminal tab, the column's drag
  // handle sets a width that outlives a reload, and Alt+I puts it all away.
  test('puts a Bead on the table, hands it over, and keeps it across tabs at the width it was given', async ({ page }) => {
    const grid = page.locator('.terminal-grid[data-workspace="terminal1"]')
    const gridBefore = await box(grid)

    await openBeadsTab(page)
    await page.click('.bead-row:has-text("Fix login bug") .bead-row-open')

    const table = page.getByRole('complementary', { name: 'Bead test-ep1.1' })
    await expect(table.locator('.bead-card-title')).toHaveText('Fix login bug')
    await expect(table).toContainText('A login survives a reload.')
    await expect(table.locator('.bead-card-fields')).toContainText('test-ep1')

    // Copy id confirms as a toast in the bottom-centre slot, and the status
    // line keeps the same event as the record.
    await table.getByRole('button', { name: 'Copy id' }).click()
    await expect(page.locator('.toast')).toHaveText('Copied test-ep1.1')
    await expect(page.locator('.status-line')).toContainText('Copied test-ep1.1')

    const send = table.getByRole('button', { name: 'Send' })
    await send.click()
    const drawer = page.getByRole('dialog', { name: 'Send to session' })
    await expect(drawer.locator('.send-drawer-reference')).toHaveText('bead test-ep1.1: Fix login bug')
    await expect(drawer.getByLabel('Message to send')).toBeFocused()

    // The card is still there beneath: the drawer ends where the column ends
    // and starts inside it, so the column was overlaid, not replaced.
    const tableBox = await box(table)
    const drawerBox = await box(drawer)
    expect(drawerBox.x).toBeGreaterThan(tableBox.x)
    expect(Math.round(drawerBox.x + drawerBox.width)).toBe(Math.round(tableBox.x + tableBox.width))

    await page.keyboard.press('Escape')
    await expect(drawer).toHaveCount(0)
    await expect(table).toBeVisible()
    await expect(send).toBeFocused()

    await page.keyboard.press('Alt+1')
    const column = page.locator('.terminal-workspace-dock[data-active="true"] .table-column')
    await expect(column.locator('.bead-card-id')).toHaveText('test-ep1.1')
    const gridAfter = await box(grid)
    const columnBox = await box(column)
    expect(gridAfter.width).toBeLessThan(gridBefore.width)
    expect(gridAfter.x + gridAfter.width).toBeLessThanOrEqual(columnBox.x + 1)

    const handle = column.locator('.table-column-handle')
    const handleBox = await box(handle)
    const grabX = handleBox.x + 2
    await page.mouse.move(grabX, handleBox.y + 200)
    await page.mouse.down()
    await page.mouse.move(grabX - 60, handleBox.y + 200, { steps: 4 })
    await page.mouse.move(grabX - 120, handleBox.y + 200, { steps: 4 })
    await page.mouse.up()
    const widened = Math.round(columnBox.width) + 120
    expect(Math.round((await box(column)).width)).toBe(widened)

    await page.reload()
    await page.waitForSelector('.dashboard')
    await openBeadsTab(page)
    await page.click('.bead-row:has-text("Fix login bug") .bead-row-open')
    const reopened = page.getByRole('complementary', { name: 'Bead test-ep1.1' })
    await expect(reopened.locator('.bead-card-title')).toHaveText('Fix login bug')
    expect(Math.round((await box(reopened)).width)).toBe(widened)

    await page.keyboard.press('Alt+i')
    await expect(reopened).toHaveCount(0)
  })

  test('follows an id inside the card and comes back', async ({ page }) => {
    await openBeadsTab(page)
    await page.click('.bead-row:has-text("Fix login bug") .bead-row-open')

    await page.click('.bead-card-section .chrote-markdown-token')

    await expect(page.locator('.bead-card-id')).toHaveText('test-ep1.2')
    await page.getByRole('button', { name: 'Back' }).click()
    await expect(page.locator('.bead-card-id')).toHaveText('test-ep1.1')
  })

  test('says what refused rather than showing a blank tab', async ({ page }) => {
    allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 503')
    await mockBeadsApiError(page)

    await openBeadsTab(page)

    await expect(page.locator('.beads-error')).toContainText('bd command not found')
  })
})
