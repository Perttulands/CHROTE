/** Camera and saved-view continuity require the actual canvas and browser layout. */
import { expect, test, type Page } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockLibraryApiRoutes } from './mock-api'

const TITLE = 'Workflow Preferences'
const OLD_SUMMARY = 'Original workflow evidence.'
const NEW_SUMMARY = 'Current workflow evidence after reload.'
const created = new Date(Date.now() - 200 * 86_400_000).toISOString()
const updated = new Date(Date.now() - 3 * 3_600_000).toISOString()

function graph(fresh = false) {
  return {
    pages: [
      { path: 'knowledge/testing.md', slug: 'testing', shelf: 'knowledge', subject: 'knowledge', title: 'Test isolation', kind: 'topic', facets: ['software'], summary: 'Testing evidence.', words: 60, created, updated, candidate: false },
      { path: 'knowledge/long.md', slug: 'long', shelf: 'knowledge', subject: 'knowledge', title: 'Long-running observations', kind: 'note', facets: ['software'], summary: '', words: 45, created, updated, candidate: false },
      { path: 'preferences/tools.md', slug: 'tools', shelf: 'preferences', subject: 'preferences', title: 'Tool Preferences', kind: 'source', facets: ['tooling'], summary: 'Preserved source.', words: 30, created, updated, candidate: false },
      { path: 'preferences/workflow.md', slug: 'workflow', shelf: 'preferences', subject: 'preferences', title: TITLE, kind: 'research', facets: ['software'], summary: fresh ? NEW_SUMMARY : OLD_SUMMARY, words: 200, created, updated, candidate: false },
    ],
    links: [['knowledge/testing.md', 'preferences/workflow.md'], ['preferences/workflow.md', 'preferences/tools.md']],
    tags: [],
  }
}

async function setup(page: Page, fresh: () => boolean = () => false) {
  await mockApiRoutes(page)
  await mockBeadsApiRoutes(page)
  await mockLibraryApiRoutes(page)
  await page.route('**/api/library/graph**', route => route.fulfill({ json: graph(fresh()) }))
  await page.goto('/')
  if (page.viewportSize()!.width <= 768) await page.getByRole('button', { name: '☰', exact: true }).click()
  await page.getByRole('button', { name: 'Brain', exact: true }).click()
  await expect(page.getByRole('button', { name: 'preferences, 2 pages' })).toBeVisible()
}

const modes = (page: Page) => page.getByRole('group', { name: 'Brain view mode' })
const continuous = (page: Page) => page.locator('.brain-continuous')
const selected = (page: Page) => page.locator('.brain-page-selection')
/** On the sky a selection is the star its label marks as current. */
const star = (page: Page) => continuous(page).locator('[aria-current="true"]')
const time = (page: Page) => page.getByRole('slider', { name: 'The moment the map is read at' })

/** These are rendered label coordinates and the visible scale, not component state. */
async function cameraGeometry(page: Page) {
  const label = continuous(page).getByRole('button', { name: TITLE, exact: true })
  await expect(label).toBeVisible()
  return {
    label: await label.evaluate(element => ({ left: (element as HTMLElement).style.left, top: (element as HTMLElement).style.top })),
    scale: await page.locator('.brain-map-scale').textContent(),
  }
}

async function chooseWorkflow(page: Page) {
  await page.getByRole('button', { name: /preferences, [12] pages?/ }).click()
  await page.locator('.library-map').getByRole('button', { name: TITLE, exact: true }).click()
  await expect(selected(page)).toContainText(TITLE)
}

async function moveMap(page: Page) {
  const surface = continuous(page)
  const before = await page.locator('.brain-map-scale').textContent()
  const beforePan = await cameraGeometry(page)
  await surface.focus()
  await page.keyboard.press('ArrowRight')
  await expect.poll(() => cameraGeometry(page)).not.toEqual(beforePan)
  await page.keyboard.press('+')
  await expect(page.locator('.brain-map-scale')).not.toHaveText(before ?? '')
  // The sky glides to a zoom; the camera worth keeping is the one it rests at.
  await expect.poll(async () => {
    const first = JSON.stringify(await cameraGeometry(page))
    await page.waitForTimeout(150)
    return JSON.stringify(await cameraGeometry(page)) === first
  }).toBe(true)
}

test('keeps selection, filters and the map camera across modes and a tab return', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 1000 })
  await setup(page)
  await page.getByRole('combobox', { name: 'Map facet' }).selectOption('software')
  await chooseWorkflow(page)
  await modes(page).getByRole('button', { name: 'Map', exact: true }).click()
  await expect(star(page)).toHaveText(TITLE)
  await expect(page.getByRole('combobox', { name: 'Map facet' })).toHaveValue('software')
  await moveMap(page)
  const camera = await cameraGeometry(page)
  await modes(page).getByRole('button', { name: 'Subjects', exact: true }).click()
  await expect(selected(page)).toContainText(TITLE)
  await expect(page.getByRole('combobox', { name: 'Map facet' })).toHaveValue('software')
  await modes(page).getByRole('button', { name: 'Map', exact: true }).click()
  await expect.poll(() => cameraGeometry(page)).toEqual(camera)
  await page.getByRole('button', { name: 'Files', exact: true }).click()
  const refreshed = page.waitForResponse(response => new URL(response.url()).pathname === '/api/library/graph')
  await page.getByRole('button', { name: 'Brain', exact: true }).click()
  await (await refreshed).finished()
  await expect(star(page)).toHaveText(TITLE)
  await expect.poll(() => cameraGeometry(page)).toEqual(camera)

  // On the sky, choosing a star reads its page beside it.
  await star(page).click()
  await expect(page.locator('.library-dive .library-body')).toContainText('Prefer small, verifiable changes.')
})

test('recalls a saved camera, selection, filters and time against fresh content after reload', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 1000 })
  let fresh = false
  await setup(page, () => fresh)
  await page.getByRole('combobox', { name: 'Map collection' }).selectOption('preferences')
  await page.getByRole('combobox', { name: 'Map facet' }).selectOption('software')
  await time(page).fill('827')
  const savedTime = await time(page).inputValue()
  await page.getByRole('checkbox', { name: /^source/ }).uncheck()
  await page.getByRole('button', { name: 'Hide knowledge on the map' }).click()
  await chooseWorkflow(page)
  await modes(page).getByRole('button', { name: 'Map', exact: true }).click()
  await moveMap(page)
  const camera = await cameraGeometry(page)
  await page.getByRole('button', { name: 'Save view 1' }).click()

  await modes(page).getByRole('button', { name: 'Subjects', exact: true }).click()
  await page.getByRole('button', { name: 'Clear selection' }).click()
  await page.getByRole('combobox', { name: 'Map collection' }).selectOption('')
  await page.getByRole('combobox', { name: 'Map facet' }).selectOption('')
  await time(page).fill('1000')
  await page.getByRole('checkbox', { name: /^source/ }).check()
  await page.getByRole('button', { name: 'Show knowledge on the map' }).click()
  await page.getByRole('button', { name: 'Restore View 1' }).click()
  await expect(modes(page).getByRole('button', { name: 'Map', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(star(page)).toHaveText(TITLE)
  await expect(page.getByRole('combobox', { name: 'Map collection' })).toHaveValue('preferences')
  await expect(page.getByRole('combobox', { name: 'Map facet' })).toHaveValue('software')
  await expect(time(page)).toHaveValue(savedTime)
  await expect(page.getByRole('checkbox', { name: /^source/ })).not.toBeChecked()
  await expect(page.getByRole('button', { name: 'Show knowledge on the map' })).toBeVisible()
  await expect.poll(() => cameraGeometry(page)).toEqual(camera)

  fresh = true
  await page.reload()
  await page.getByRole('button', { name: 'Brain', exact: true }).click()
  await page.getByRole('button', { name: 'Restore View 1' }).click()
  await expect(star(page)).toHaveText(TITLE)
  await expect(time(page)).toHaveValue(savedTime)
  await expect.poll(() => cameraGeometry(page)).toEqual(camera)
  await modes(page).getByRole('button', { name: 'Subjects', exact: true }).click()
  await expect(selected(page)).toContainText(NEW_SUMMARY)
  await expect(selected(page)).not.toContainText(OLD_SUMMARY)
})

test('restores a saved Subjects camera after another subject and page were selected', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 1000 })
  await setup(page)
  await chooseWorkflow(page)
  const nodes = page.locator('.library-map-nodes')
  const original = await nodes.evaluate(element => (element as HTMLElement).style.transform)
  await page.getByRole('img', { name: 'The map' }).hover({ position: { x: 20, y: 20 } })
  await page.mouse.wheel(0, -250)
  await expect.poll(() => nodes.evaluate(element => (element as HTMLElement).style.transform)).not.toBe(original)
  const camera = await nodes.evaluate(element => (element as HTMLElement).style.transform)
  await page.getByRole('button', { name: 'Save view 2' }).click()
  await page.getByRole('button', { name: 'knowledge, 2 pages' }).click()
  await page.locator('.library-map').getByRole('button', { name: 'Test isolation', exact: true }).click()
  await expect(selected(page)).toContainText('Test isolation')
  await page.getByRole('button', { name: 'Restore View 2' }).click()
  await expect(modes(page).getByRole('button', { name: 'Subjects', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(selected(page)).toContainText(TITLE)
  await expect(page.locator('.brain-breadcrumb')).toHaveText('preferences / preferences')
  await expect.poll(() => nodes.evaluate(element => (element as HTMLElement).style.transform)).toBe(camera)
})

test('keeps mode, time and all four view slots reachable on a phone', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await setup(page)
  const footer = page.locator('footer[aria-label="Brain view controls"]')
  await modes(page).getByRole('button', { name: 'Map', exact: true }).click()
  for (let index = 1; index <= 4; index++) {
    const slot = page.getByRole('button', { name: `Save view ${index}` })
    await expect(slot).toBeInViewport()
    const box = await slot.boundingBox()
    expect(box!.x).toBeGreaterThanOrEqual(0)
    expect(box!.x + box!.width).toBeLessThanOrEqual(390)
  }
  await expect(time(page)).toBeInViewport()
  await page.getByRole('button', { name: 'Save view 4' }).click()
  await modes(page).getByRole('button', { name: 'Subjects', exact: true }).click()
  await page.getByRole('button', { name: 'Restore View 4' }).click()
  await expect(modes(page).getByRole('button', { name: 'Map', exact: true })).toHaveAttribute('aria-pressed', 'true')
  await expect(footer).toBeInViewport()

  // A long summary scrolls inside its copy area; it cannot push the actions
  // below the phone viewport or require scrolling the copy to reach them.
  const longGraph = graph()
  longGraph.pages[3].summary = 'Long evidence for this subject. '.repeat(40)
  await page.route('**/api/library/graph**', route => route.fulfill({ json: longGraph }))
  const refreshed = page.waitForResponse(response => new URL(response.url()).pathname === '/api/library/graph')
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await (await refreshed).finished()
  await modes(page).getByRole('button', { name: 'Subjects', exact: true }).click()
  await chooseWorkflow(page)
  const copy = selected(page).locator('.brain-selection-copy')
  await expect.poll(() => copy.evaluate(element => element.scrollHeight > element.clientHeight)).toBe(true)
  expect(await copy.evaluate(element => element.scrollTop)).toBe(0)
  const open = selected(page).getByRole('button', { name: 'Open page', exact: true })
  await expect(open).toBeInViewport()
  await expect(selected(page).getByRole('button', { name: 'Clear selection', exact: true })).toBeInViewport()
  await open.click()
  await expect(page.locator('.library-dive .library-body')).toContainText('Prefer small, verifiable changes.')
})
