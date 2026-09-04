/**
 * The Beads tab, the Bead card on the table, and the drawer over it
 * (beads: chrote-5grx.15, chrote-5grx.47, chrote-5grx.74).
 */

import { test, expect, allowBrowserConsoleMessage, type Locator, type Page } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockBeadsApiError, mockBeadsWork } from './mock-api'

async function openBeadsTab(page: Page) {
  await page.click('.tab:has-text("Beads")')
  await page.waitForSelector('.beads-view')
}

async function box(locator: Locator) {
  const value = await locator.boundingBox()
  if (!value) throw new Error('expected a rendered bounding box')
  return value
}

async function expectFlowNodeCentred(page: Page, id: string) {
  const idLabel = page.locator('.bead-flow-node-id').getByText(id, { exact: true })
  const node = idLabel.locator('../..')
  await expect(node).toBeVisible()
  const [nodeBox, flowBox] = [await box(node), await box(page.locator('.bead-flow-surface'))]
  expect(Math.abs(nodeBox.x + nodeBox.width / 2 - (flowBox.x + flowBox.width / 2))).toBeLessThan(6)
  expect(Math.abs(nodeBox.y + nodeBox.height / 2 - (flowBox.y + flowBox.height / 2))).toBeLessThan(6)
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
    await expect(page.locator('.bead-map-acceptance').first()).toContainText('Every surface reads the same way')
    await expect(page.locator('.bead-row-blocked').first()).toContainText('blocked by test-ep1.2')
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

  test('folds an epic from its title and narrows by search', async ({ page }) => {
    await openBeadsTab(page)

    const epic = page.locator('.bead-row', { hasText: 'One interaction language' })
    await expect(epic.locator('.bead-row-fold')).toHaveText('▾3')
    await epic.locator('.bead-row-title').click()
    await expect(page.locator('.bead-row', { hasText: 'Fix login bug' })).toHaveCount(0)
    await expect(epic.locator('.bead-row-fold')).toHaveText('▸3')
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

  test('opens linked rows at their place in Flow and disables an orphan', async ({ page }) => {
    await openBeadsTab(page)

    const orphan = page.locator('.bead-row', { has: page.getByText('Unlinked note', { exact: true }) })
    await orphan.click({ button: 'right' })
    const orphanAction = page.getByRole('menu', { name: 'Actions for test-alone' })
      .getByRole('menuitem', { name: /Open in Flow/ })
    await expect(orphanAction).toBeDisabled()
    await expect(orphanAction).toHaveAttribute('title', 'No linked work')
    await page.keyboard.press('Escape')

    const child = page.locator('.bead-row', { has: page.getByText('Fix login bug', { exact: true }) })
    await child.click({ button: 'right' })
    await page.getByRole('menu', { name: 'Actions for test-ep1.1' })
      .getByRole('menuitem', { name: 'Open in Flow' }).click()
    await expect(page.getByRole('tab', { name: 'Flow' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('.bead-flow-epic-name')).toContainText('Linked flow · test-ep1.1')
    await expectFlowNodeCentred(page, 'test-ep1.1')

    await page.getByRole('tab', { name: 'Map' }).click()
    const epic = page.locator('.bead-row', { has: page.getByText('One interaction language', { exact: true }) })
    await epic.click({ button: 'right' })
    await page.getByRole('menu', { name: 'Actions for test-ep1' })
      .getByRole('menuitem', { name: 'Open in Flow' }).click()
    await expectFlowNodeCentred(page, 'test-ep1')

    // The linked pair lives in another project and under no epic. Opening one
    // from All selects that project and constructs its dependency flow.
    await page.getByRole('tab', { name: 'Map' }).click()
    await page.getByRole('button', { name: 'All', exact: true }).click()
    const standalone = page.locator('.bead-row', { has: page.getByText('Finish loose work', { exact: true }) })
    await standalone.click({ button: 'right' })
    await page.getByRole('menu', { name: 'Actions for other-b' })
      .getByRole('menuitem', { name: 'Open in Flow' }).click()

    await expect(page.getByRole('button', { name: 'other', exact: true })).toHaveClass(/active/)
    await expect(page.locator('.bead-flow-node')).toHaveCount(2)
    await expectFlowNodeCentred(page, 'other-b')
  })

  test('splits ready from in progress, and lists what has gone stale', async ({ page }) => {
    await openBeadsTab(page)

    await page.click('.beads-view-tab:has-text("Open")')
    const ready = page.locator('.beads-column').first()
    const inProgress = page.locator('.beads-column').nth(1)
    await expect(ready.getByRole('heading')).toHaveText('Ready to start')
    await expect(ready).toContainText('Fix login bug')
    await expect(inProgress).toContainText('Add dark mode')
    await expect(ready).not.toContainText('Blocked by external API')

    await page.click('.beads-view-tab:has-text("Stale")')
    await expect(page.locator('.bead-row')).toHaveCount(1)
    await expect(page.locator('.bead-row')).toContainText('Blocked by external API')
    await expect(page.locator('.bead-row-age')).toContainText('days')
  })

  test('explores templates and loads closed work only when asked', async ({ page }) => {
    const closedRequests: string[] = []
    page.on('request', request => {
      if (request.url().includes('/api/beads/closed')) closedRequests.push(request.url())
    })
    await page.reload()
    await page.waitForSelector('.dashboard')
    await openBeadsTab(page)

    expect(closedRequests).toHaveLength(0)
    await expect(page.getByText('Completed feature', { exact: true })).toHaveCount(0)

    await page.getByRole('button', { name: 'test', exact: true }).click()
    await expect(page.getByText('Formulas', { exact: true })).toBeVisible()
    await expect(page.getByText('Template protos', { exact: true })).toBeVisible()
    await expect(page.getByText('Molecules', { exact: true })).toBeVisible()

    await page.getByRole('button', { name: 'release', exact: true }).click()
    const formula = page.locator('.beads-template-detail')
    await expect(formula.getByRole('heading', { name: 'release', level: 1 })).toBeVisible()
    await expect(formula).toContainText('/code/test-project/.beads/formulas/release.formula.toml')
    await expect(formula).toContainText('Build the dashboard')
    await expect(formula).toContainText('Depends on')
    await expect(formula.locator('button')).toHaveCount(0)

    await page.getByText('September release', { exact: true }).click()
    const molecule = page.locator('.beads-template-detail')
    await expect(molecule).toContainText('Dependencies')
    await expect(molecule).toContainText('production')

    await page.getByRole('button', { name: 'All', exact: true }).click()
    await page.getByRole('tab', { name: 'Closed' }).click()
    await expect(page.getByText('Completed feature', { exact: true })).toBeVisible()
    await expect(page.getByText('Archived quiet-store task', { exact: true })).toBeVisible()
    expect(closedRequests).toHaveLength(3)

    await page.getByLabel('Search closed Beads').fill('quiet-store')
    await expect(page.getByText('Archived quiet-store task', { exact: true })).toBeVisible()
    await expect(page.getByText('Completed feature', { exact: true })).toHaveCount(0)

    await page.getByRole('tab', { name: 'Map' }).click()
    await page.getByRole('tab', { name: 'Closed' }).click()
    expect(closedRequests).toHaveLength(3)
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

  // The flow of an epic: two chains that can run at once, the join that waits
  // for both, travel from the keyboard, and a stable viewport while the table
  // opens and closes beside it.
  test('lays an epic out in waves and travels it Bead to Bead', async ({ page }) => {
    await openBeadsTab(page)
    await page.click('.beads-view-tab:has-text("Flow")')
    await page.getByLabel('Epic to flow').selectOption({ label: 'test-ep2 · Ship the reading room' })

    const nodes = page.locator('.bead-flow-node')
    await expect(nodes).toHaveCount(5)

    // Two chains nobody blocks stand in the same column, one above the other;
    // what waits for one of them stands in the next column.
    const measure = page.locator('.bead-flow-node', { hasText: 'Measure the shelves' })
    const draw = page.locator('.bead-flow-node', { hasText: 'Draw the shelves' })
    const index = page.locator('.bead-flow-node', { hasText: 'Index the pages' })
    const [measureBox, drawBox, indexBox] = [await box(measure), await box(draw), await box(index)]
    expect(measureBox.x).toBe(drawBox.x)
    expect(drawBox.y).toBeGreaterThan(measureBox.y)
    expect(indexBox.x).toBeGreaterThan(measureBox.x)

    // The join waits for the end of both chains: two lines arrive at it.
    await expect(page.locator('.bead-flow-edge')).toHaveCount(4)

    // Right travels a wave, down travels the column.
    await measure.focus()
    await page.keyboard.press('ArrowRight')
    await expect(index).toBeFocused()
    await page.keyboard.press('ArrowDown')
    await expect(page.locator('.bead-flow-node', { hasText: 'Search the index' })).toBeFocused()

    // A click puts the Bead on the table without moving work under the pointer,
    // even though the table narrows the drawing's box beside it.
    const measureBefore = await box(measure)
    await measure.click()
    await expect(page.getByRole('complementary', { name: 'Bead test-ep2.1' })).toBeVisible()
    const measureWithTable = await box(measure)
    expect(Math.abs(measureWithTable.x - measureBefore.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(measureWithTable.y - measureBefore.y)).toBeLessThanOrEqual(1)

    await page.keyboard.press('Alt+i')
    await expect(page.getByRole('complementary', { name: 'Bead test-ep2.1' })).toHaveCount(0)
    const measureAfterClose = await box(measure)
    expect(Math.abs(measureAfterClose.x - measureBefore.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(measureAfterClose.y - measureBefore.y)).toBeLessThanOrEqual(1)

    // Manual zoom and pan survive selection. Fit remains the explicit way to
    // restore the layout's original transform.
    const flow = page.locator('[data-ui="beads.flow"]')
    const flowBox = await box(flow)
    await page.mouse.move(flowBox.x + 50, flowBox.y + 50)
    await page.mouse.wheel(0, -160)
    await page.mouse.move(flowBox.x + 80, flowBox.y + 80)
    await page.mouse.down()
    await page.mouse.move(flowBox.x + 120, flowBox.y + 105, { steps: 3 })
    await page.mouse.up()
    const canvas = page.locator('.bead-flow-canvas')
    const movedTransform = await canvas.evaluate(element => (element as HTMLElement).style.transform)
    expect(movedTransform).not.toBe('translate(0px, 0px) scale(1)')

    const movedNodeBefore = await box(measure)
    await measure.click()
    await expect(page.getByRole('complementary', { name: 'Bead test-ep2.1' })).toBeVisible()
    const movedNodeAfter = await box(measure)
    expect(Math.abs(movedNodeAfter.x - movedNodeBefore.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(movedNodeAfter.y - movedNodeBefore.y)).toBeLessThanOrEqual(1)
    expect(await canvas.evaluate(element => (element as HTMLElement).style.transform)).toBe(movedTransform)

    await page.getByRole('button', { name: 'Fit' }).click()
    await expect(canvas).toHaveCSS('transform', 'matrix(1, 0, 0, 1, 0, 0)')
  })

  test('keeps the epic already on the table when one of its Flow children opens', async ({ page }) => {
    // This journey needs the issue route to echo the requested row. The shared
    // card fixture describes only test-ep1.1, which would erase the table epic
    // before Flow could use it as the initial graph choice.
    await page.route('**/api/beads/issue**', async route => {
      const id = new URL(route.request().url()).searchParams.get('id')
      const row = mockBeadsWork.data.beads.find(bead => bead.id === id)
      if (!row) return route.fallback()
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          data: {
            projectPath: mockBeadsWork.data.projectPath,
            bead: { ...row, parents: [], children: [], blockedBy: [], blocks: [] },
          },
        }),
      })
    })

    await openBeadsTab(page)
    await page.locator('.bead-row', { has: page.getByText('Ship the reading room', { exact: true }) })
      .locator('.bead-row-open').click()
    await expect(page.getByRole('complementary', { name: 'Bead test-ep2' })).toBeVisible()

    await page.getByRole('tab', { name: 'Flow' }).click()
    const picker = page.getByLabel('Epic to flow')
    await expect(picker.locator('option:checked')).toHaveText('test-ep2 · Ship the reading room')
    await expect(page.locator('.bead-flow-node')).toHaveCount(5)

    const child = page.locator('.bead-flow-node', { hasText: 'Measure the shelves' })
    const before = await box(child)
    await child.click()
    await expect(page.getByRole('complementary', { name: 'Bead test-ep2.1' })).toBeVisible()

    await expect(picker.locator('option:checked')).toHaveText('test-ep2 · Ship the reading room')
    await expect(page.locator('.bead-flow-node')).toHaveCount(5)
    const after = await box(child)
    expect(Math.abs(after.x - before.x)).toBeLessThanOrEqual(1)
    expect(Math.abs(after.y - before.y)).toBeLessThanOrEqual(1)
  })

  test('follows an id inside the card and comes back', async ({ page }) => {
    await openBeadsTab(page)
    await page.click('.bead-row:has-text("Fix login bug") .bead-row-open')

    await page.click('.bead-card-section .chrote-markdown-token')

    const followed = page.getByRole('complementary', { name: 'Bead test-ep1.2' })
    await expect(followed).toBeVisible()
    await followed.getByRole('button', { name: 'Back' }).click()
    await expect(page.getByRole('complementary', { name: 'Bead test-ep1.1' })).toBeVisible()
  })

  test('says what refused rather than showing a blank tab', async ({ page }) => {
    allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 503')
    await mockBeadsApiError(page)

    await openBeadsTab(page)

    await expect(page.locator('.beads-error')).toContainText('bd command not found')
  })
})
