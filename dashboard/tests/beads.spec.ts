/**
 * The Beads tab: every Bead on the host in one table, sliced by search,
 * project, status and days, with the selected Bead's work graph and text
 * beside it; and the Bead card on the table and the drawer over it
 * (beads: chrote-qtoq.2, chrote-qtoq.3, chrote-5grx.15, chrote-5grx.47, chrote-5grx.74).
 */

import { test, expect, allowBrowserConsoleMessage, type Locator, type Page } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockBeadsApiError, mockHostStores } from './mock-api'

const ROW = '.beads-row[data-ui="beads.row"]'

async function openBeadsTab(page: Page) {
  await page.click('.tab:has-text("Beads")')
  await page.waitForSelector(`.beads-view ${ROW}`)
}

async function box(locator: Locator) {
  const value = await locator.boundingBox()
  if (!value) throw new Error('expected a rendered bounding box')
  return value
}

const row = (page: Page, title: string) => page.locator(ROW, { has: page.locator('.beads-cell-title', { hasText: title }) })
const ids = (page: Page) => page.locator(`${ROW} .beads-cell-id`).allTextContents()
const count = (page: Page) => page.locator('.beads-count')

test.describe('Beads', () => {
  // The tab holds the rail, the table, the graph and the Clerk's column side
  // by side; this is the desktop width it is drawn for.
  test.use({ viewport: { width: 1600, height: 900 } })

  test.beforeEach(async ({ page }) => {
    await mockApiRoutes(page)
    await mockBeadsApiRoutes(page)
    await page.goto('/')
    await page.waitForSelector('.dashboard')
  })

  test('slices every store by search, project, status and days', async ({ page }) => {
    await openBeadsTab(page)
    await expect(count(page)).toHaveText('16 of 16 Beads')
    await expect(page.locator('.beads-footer')).toContainText('3 stores read')

    // Ids and titles answer as the operator types; the text a row does not
    // carry is searched by the server and joined in.
    const search = page.getByLabel('Search Beads')
    await search.fill('login')
    await expect.poll(() => ids(page)).toEqual(['test-ep1.1'])
    await search.fill('drops the session')
    await expect.poll(() => ids(page)).toEqual(['test-ep1.1'])
    await search.fill('')
    await expect(count(page)).toHaveText('16 of 16 Beads')

    // A project's name keeps it alone; its box toggles it among the others.
    const projects = page.getByRole('group', { name: 'Projects' })
    await projects.getByRole('button', { name: 'other', exact: true }).click()
    await expect.poll(async () => (await ids(page)).sort()).toEqual(['other-a', 'other-b'])
    await projects.getByLabel('Keep quiet').check()
    await expect(count(page)).toHaveText('3 of 16 Beads')
    await page.locator('.beads-facet-all').first().click()
    await expect(count(page)).toHaveText('16 of 16 Beads')

    const statuses = page.getByRole('group', { name: 'Statuses' })
    await statuses.getByRole('button', { name: /closed/ }).click()
    await expect.poll(async () => (await ids(page)).sort()).toEqual(['quiet-done', 'test-ep1.4', 'test-ep2.6'])
    await statuses.getByRole('button', { name: /closed/ }).click()

    // Columns sort in their natural direction first, then the other way.
    const sizeHeader = page.getByRole('columnheader', { name: /Size/ })
    await sizeHeader.getByRole('button').click()
    await expect(sizeHeader).toHaveAttribute('aria-sort', 'descending')
    expect((await ids(page))[0]).toBe('test-ep2.6')
    const idleHeader = page.getByRole('columnheader', { name: /Idle/ })
    await idleHeader.getByRole('button').click()
    await expect(idleHeader).toHaveAttribute('aria-sort', 'descending')
    expect((await ids(page)).slice(0, 3).sort()).toEqual(['quiet-done', 'test-ep1.3', 'test-ep2.6'])

    // A drag across the strip keeps the Beads of those days, and the label
    // names the range until it is cleared.
    const strip = page.locator('.beads-strip-hit')
    const stripBox = await box(strip)
    await page.mouse.move(stripBox.x + 2, stripBox.y + stripBox.height / 2)
    await page.mouse.down()
    await page.mouse.move(stripBox.x + stripBox.width * 0.3, stripBox.y + stripBox.height / 2, { steps: 5 })
    await page.mouse.up()
    await expect(page.locator('.beads-range')).toHaveClass(/set/)
    await expect(page.locator('.beads-range')).toContainText(' to ')
    await expect(count(page)).not.toHaveText('16 of 16 Beads')
    await expect.poll(() => new URL(page.url()).searchParams.get('from')).not.toBeNull()
    await page.getByRole('button', { name: 'Clear the date range' }).click()
    await expect(page.locator('.beads-range')).toHaveText('All days')
    await expect(count(page)).toHaveText('16 of 16 Beads')
  })

  test('draws the selected row\'s work graph beside the table and selects a row from a node', async ({ page }) => {
    await page.setViewportSize({ width: 1920, height: 900 })
    await openBeadsTab(page)
    await row(page, 'Fix login bug').click()
    await expect(row(page, 'Fix login bug')).toHaveAttribute('aria-selected', 'true')

    const graph = page.locator('.beads-graph')
    await expect(graph.getByRole('button', { name: 'test-ep1: One interaction language, open' })).toBeVisible()
    await expect(graph.locator('.beads-node')).toHaveCount(4)
    const selected = graph.getByRole('button', { name: 'test-ep1.1: Fix login bug' })
    await expect(selected).toHaveAttribute('aria-pressed', 'true')
    await expect(graph.getByRole('button', { name: 'test-ep1.4: Completed feature' })).toHaveClass(/closed/)
    await expect(graph.getByRole('button', { name: 'test-ep1.3: Blocked by external API' })).toHaveClass(/blocked/)
    await expect(graph.locator('.beads-graph-legend')).toContainText('test')
    // Its text is read in the view, beside the graph.
    await expect(page.locator('.beads-text')).toContainText('Fix login bug')
    await expect(page.locator('.beads-text')).toContainText('Description')

    // Every card is drawn inside the pane, beside the table, not over it.
    const [tableBox, canvasBox] = [await box(page.locator('.beads-table')), await box(graph.locator('.beads-graph-canvas'))]
    expect(canvasBox.x).toBeGreaterThanOrEqual(tableBox.x + tableBox.width - 1)
    for (const node of await graph.locator('.beads-node').all()) {
      const nodeBox = await box(node)
      expect(nodeBox.x).toBeGreaterThanOrEqual(canvasBox.x - 1)
      expect(nodeBox.x + nodeBox.width).toBeLessThanOrEqual(canvasBox.x + canvasBox.width + 1)
    }

    // A node selects its row, and the drawing stays where it was.
    const before = await box(selected)
    await graph.getByRole('button', { name: 'test-ep1.2: Add dark mode' }).click()
    await expect(row(page, 'Add dark mode')).toHaveAttribute('aria-selected', 'true')
    const after = await box(selected)
    expect(Math.abs(after.x - before.x)).toBeLessThanOrEqual(1)

    // The layout switch shows the table or the graph alone.
    await page.getByRole('tab', { name: 'Table' }).click()
    await expect(graph).toHaveCount(0)
    await page.getByRole('tab', { name: 'Graph' }).click()
    await expect(page.locator('.beads-table')).toHaveCount(0)
    await expect(page.locator('.beads-graph').getByRole('button', { name: 'test-ep1.2: Add dark mode' })).toHaveAttribute('aria-pressed', 'true')
    await page.getByRole('tab', { name: 'Split' }).click()
    await expect(page.locator('.beads-table')).toBeVisible()
    await expect(page.locator('.beads-graph')).toBeVisible()
  })

  test('opens the slice a copied link names in a fresh tab', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await openBeadsTab(page)
    await page.getByLabel('Search Beads').fill('shelves')
    await page.getByRole('columnheader', { name: /Title/ }).getByRole('button').click()
    await row(page, 'Measure the shelves').click()
    await page.getByRole('tab', { name: 'Graph' }).click()
    await page.getByRole('button', { name: 'Copy view link' }).click()
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toContain('sel=test-ep2.1')
    const link = await page.evaluate(() => navigator.clipboard.readText())
    expect(new URL(link).searchParams.get('tab')).toBe('beads')

    const fresh = await context.newPage()
    await mockApiRoutes(fresh)
    await mockBeadsApiRoutes(fresh)
    await fresh.goto(link)
    await expect(fresh.getByLabel('Search Beads')).toHaveValue('shelves')
    await expect(fresh.getByRole('tab', { name: 'Graph' })).toHaveAttribute('aria-selected', 'true')
    await expect(fresh.locator('.beads-graph').getByRole('button', { name: 'test-ep2.1: Measure the shelves' })).toHaveAttribute('aria-pressed', 'true')
    await fresh.getByRole('tab', { name: 'Table' }).click()
    await expect(fresh.getByRole('columnheader', { name: /Title/ })).toHaveAttribute('aria-sort', 'ascending')
    await expect.poll(() => ids(fresh)).toEqual(['test-ep2.2', 'test-ep2.1'])
    await expect(row(fresh, 'Measure the shelves')).toHaveAttribute('aria-selected', 'true')
    await fresh.close()
  })

  // The table's geometry is the browser's: long titles wrap inside the title
  // column, every column stays inside the pane beside the rail and the Clerk,
  // and Split puts the graph beneath the table once both no longer fit side by
  // side. Narrower still, the table drops Updated, which Idle says, rather than
  // scroll sideways or squeeze a column; every id stays whole throughout.
  test('keeps the table readable beside the rail, the graph and the Clerk', async ({ page }, testInfo) => {
    const title = 'Make every configured project and all its long-running agent work readable beside the graph and Clerk without losing the end of this title'
    await mockBeadsApiRoutes(page, {
      hostStores: mockHostStores().map(store => ({
        ...store,
        rows: store.rows.map(item => item.id === 'test-ep1.1' ? { ...item, title } : item),
      })),
    })
    await openBeadsTab(page)
    const table = page.locator('.beads-table')
    const canvas = page.locator('.beads-graph-canvas')
    const titleCell = page.locator(ROW, { hasText: 'test-ep1.1' }).locator('.beads-cell-title')
    const sideways = () => table.evaluate(node => node.scrollWidth - node.clientWidth)
    const updated = page.getByRole('columnheader', { name: /Updated/ })
    for (const [width, layout] of [[2048, 'beside'], [1600, 'beneath'], [1280, 'compact']] as const) {
      await page.setViewportSize({ width, height: 900 })
      await expect(titleCell).toBeVisible()
      await expect.poll(sideways).toBeLessThanOrEqual(1)
      if (layout === 'compact') await expect(updated).toHaveCount(0)
      else await expect(updated).toHaveCount(1)
      const [tableBox, canvasBox, cellBox] = [await box(table), await box(canvas), await box(titleCell)]
      if (layout === 'beside') expect(canvasBox.x).toBeGreaterThanOrEqual(tableBox.x + tableBox.width - 1)
      else expect(canvasBox.y).toBeGreaterThanOrEqual(tableBox.y + tableBox.height - 1)
      expect(cellBox.x + cellBox.width).toBeLessThanOrEqual(tableBox.x + tableBox.width + 1)
      for (const cell of await page.locator(`${ROW} .beads-cell-id`).all()) {
        expect(await cell.evaluate(node => node.scrollWidth - node.clientWidth)).toBeLessThanOrEqual(0)
      }
      const metrics = await titleCell.evaluate(node => ({
        lines: node.getBoundingClientRect().height / parseFloat(getComputedStyle(node).lineHeight),
        visibleWidth: node.clientWidth,
        fullWidth: node.scrollWidth,
      }))
      expect(metrics.lines).toBeGreaterThan(1.5)
      expect(metrics.fullWidth).toBeLessThanOrEqual(metrics.visibleWidth + 1)
      // No status or number is cut short at any width.
      for (const cell of await page.locator(`${ROW} .beads-cell-status, ${ROW} .beads-cell-number`).all()) {
        expect(await cell.evaluate(node => node.scrollWidth - node.clientWidth)).toBeLessThanOrEqual(0)
      }
      await page.screenshot({ path: testInfo.outputPath(`beads-${width}.png`) })
    }
  })

  test('opens the Beads column from any tab and puts its row on the table', async ({ page }) => {
    await page.keyboard.press('Alt+b')
    const column = page.getByRole('complementary', { name: 'Beads column' })
    await expect(column).toBeVisible()
    const testStore = column.getByRole('heading', { name: 'test', level: 3 }).locator('..')
    await expect(testStore).toBeVisible()
    const stages = testStore.getByRole('heading', { level: 4 })
    await expect(stages).toHaveText(['In progress', 'Ready'])

    await page.keyboard.press('Escape')
    await expect(column).toHaveCount(0)

    await page.getByRole('button', { name: 'Agents', exact: true }).click()
    await page.keyboard.press('Alt+b')
    await column.getByRole('button', { name: /test-ep1\.1/ }).click()
    await expect(page.getByRole('complementary', { name: 'Bead test-ep1.1' })).toContainText('Fix login bug')
  })

  // The row's menu from a real right-click, and the copy it runs landing on
  // the clipboard from a menu click.
  test('copies a Bead\'s id from its row\'s menu', async ({ page, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.evaluate(() => navigator.clipboard.writeText('before-copy'))
    await openBeadsTab(page)

    await row(page, 'Fix login bug').click({ button: 'right' })
    const menu = page.getByRole('menu', { name: 'Actions for test-ep1.1' })
    await menu.getByRole('menuitem', { name: 'Copy id', exact: true }).click()

    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('test-ep1.1')
    await expect(menu).toHaveCount(0)
  })

  // The right edge, end to end: a Bead goes on the table from its row, the
  // drawer lies over the table's column and gives it back on Escape, the same
  // Bead is in a column beside the tiles on a terminal tab, the column's drag
  // handle sets a width that outlives a reload, and Alt+I puts it all away.
  test('puts a Bead on the table, hands it over, and keeps it across tabs at the width it was given', async ({ page, context }) => {
    let detailReads = 0
    page.on('request', request => {
      const url = new URL(request.url())
      if (url.pathname === '/api/beads/issue' && url.searchParams.get('id') === 'test-ep1.1') detailReads += 1
    })
    await context.grantPermissions(['clipboard-read', 'clipboard-write'])
    await page.evaluate(() => navigator.clipboard.writeText('before-copy'))
    const grid = page.locator('.terminal-grid[data-workspace="terminal1"]')
    const gridBefore = await box(grid)

    await openBeadsTab(page)
    await row(page, 'Fix login bug').dblclick()

    const table = page.getByRole('complementary', { name: 'Bead test-ep1.1' })
    await expect(table.locator('.bead-card-title')).toHaveText('Fix login bug')
    await expect(table).toContainText('A login survives a reload.')
    await expect(table.locator('.bead-card-fields')).toContainText('test-ep1')
    // StrictMode and retained surfaces share one applied card projection.
    expect(detailReads).toBe(1)

    // Read the clipboard result; announcement emission belongs to the unit tests.
    await table.getByRole('button', { name: 'Copy id' }).click()
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('test-ep1.1')

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
    expect(detailReads).toBe(1)
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
    // The keyboard opens it too: select, then Enter.
    await row(page, 'Fix login bug').click()
    await page.keyboard.press('Enter')
    const reopened = page.getByRole('complementary', { name: 'Bead test-ep1.1' })
    await expect(reopened.locator('.bead-card-title')).toHaveText('Fix login bug')
    expect(Math.round((await box(reopened)).width)).toBe(widened)

    await page.keyboard.press('Alt+i')
    await expect(reopened).toHaveCount(0)
  })

  test('follows an id inside the card and comes back', async ({ page }) => {
    await openBeadsTab(page)
    await row(page, 'Fix login bug').dblclick()

    await page.click('.bead-card-section .chrote-markdown-token')

    const followed = page.getByRole('complementary', { name: 'Bead test-ep1.2' })
    await expect(followed).toBeVisible()
    await followed.getByRole('button', { name: 'Back' }).click()
    await expect(page.getByRole('complementary', { name: 'Bead test-ep1.1' })).toBeVisible()
  })
})

// Install the refusal before first opening Beads, when its first read starts.
test('Beads says what refused rather than showing a blank tab', async ({ page }) => {
  allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 503')
  await mockApiRoutes(page, { overrides: mockBeadsApiError })
  const hostResponse = page.waitForResponse('**/api/beads/host**')
  await page.goto('/')
  await page.click('.tab:has-text("Beads")')
  expect((await hostResponse).status()).toBe(503)
  await expect(page.locator('.beads-footer')).toContainText('bd command not found')
  await expect(page.locator('.beads-empty')).toContainText('No Beads in any store this host can read.')
})
