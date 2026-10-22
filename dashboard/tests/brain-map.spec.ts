/** The sky's camera, its stars and the phone list need the actual canvas and browser layout. */
import { expect, test, type Page } from './fixtures'
import { mockApiRoutes, mockBeadsApiRoutes, mockLibraryApiRoutes } from './mock-api'

const TITLE = 'Workflow Preferences'
const created = new Date(Date.now() - 200 * 86_400_000).toISOString()
const updated = new Date(Date.now() - 3 * 3_600_000).toISOString()

function graph() {
  return {
    pages: [
      { path: 'knowledge/testing.md', slug: 'testing', shelf: 'knowledge', subject: 'knowledge', title: 'Test isolation', kind: 'topic', facets: ['software'], summary: 'Testing evidence.', words: 60, created, updated, candidate: false },
      { path: 'knowledge/long.md', slug: 'long', shelf: 'knowledge', subject: 'knowledge', title: 'Long-running observations', kind: 'note', facets: ['software'], summary: '', words: 45, created, updated, candidate: false },
      { path: 'preferences/tools.md', slug: 'tools', shelf: 'preferences', subject: 'preferences', title: 'Tool Preferences', kind: 'source', facets: ['tooling'], summary: 'Preserved source.', words: 30, created, updated, candidate: false },
      { path: 'preferences/workflow.md', slug: 'workflow', shelf: 'preferences', subject: 'preferences', title: TITLE, kind: 'research', facets: ['software'], summary: 'Workflow evidence.', words: 200, created, updated, candidate: false },
    ],
    links: [['knowledge/testing.md', 'preferences/workflow.md'], ['preferences/workflow.md', 'preferences/tools.md']],
    tags: [],
  }
}

async function setup(page: Page) {
  await mockApiRoutes(page)
  await mockBeadsApiRoutes(page)
  await mockLibraryApiRoutes(page)
  await page.route('**/api/library/graph**', route => route.fulfill({ json: graph() }))
  await page.goto('/')
  if (page.viewportSize()!.width <= 768) await page.getByRole('button', { name: '☰', exact: true }).click()
  await page.getByRole('button', { name: 'Brain', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Brain map' })).toBeVisible()
}

const continuous = (page: Page) => page.locator('.brain-continuous')
/** On the sky the page in hand is the star its label marks as current. */
const star = (page: Page) => continuous(page).locator('[aria-current="true"]')
const time = (page: Page) => page.getByRole('slider', { name: 'The moment the map is read at' })
const reader = (page: Page) => page.locator('.library-dive .library-body')

/** These are rendered label coordinates and the visible scale, not component state. */
async function cameraGeometry(page: Page) {
  const label = continuous(page).getByRole('button', { name: TITLE, exact: true })
  await expect(label).toBeVisible()
  return {
    label: await label.evaluate(element => ({ left: (element as HTMLElement).style.left, top: (element as HTMLElement).style.top })),
    scale: await page.locator('.brain-map-scale').textContent(),
  }
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

test('keeps the camera across a tab return, reads a star beside the sky, and Fit takes in the whole', async ({ page }) => {
  await page.setViewportSize({ width: 1600, height: 1000 })
  await setup(page)
  await moveMap(page)
  const camera = await cameraGeometry(page)

  await page.getByRole('button', { name: 'Files', exact: true }).click()
  await page.getByRole('button', { name: 'Brain', exact: true }).click()
  await expect.poll(() => cameraGeometry(page)).toEqual(camera)

  // A star opens its page beside the sky, and its system comes into focus.
  await continuous(page).getByRole('button', { name: TITLE, exact: true }).click()
  await expect(reader(page)).toContainText('Prefer small, verifiable changes.')
  await expect(star(page)).toHaveText(TITLE)
  await page.getByRole('button', { name: /^Close/ }).click()
  await expect(reader(page)).toHaveCount(0)

  await page.getByRole('button', { name: 'Fit', exact: true }).click()
  await expect.poll(() => cameraGeometry(page)).not.toEqual(camera)
})

test('lists the subjects on a phone, opens a page from the list, and keeps the time line in reach', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await setup(page)
  await expect(continuous(page)).toHaveCount(0)
  await expect(time(page)).toBeInViewport()

  const list = page.getByLabel('Subjects', { exact: true })
  const subject = list.getByRole('button', { name: /^preferences/ })
  await expect(subject).toHaveAttribute('aria-expanded', 'false')
  await subject.click()
  await expect(subject).toHaveAttribute('aria-expanded', 'true')
  await list.getByRole('button', { name: TITLE, exact: true }).click()
  await expect(reader(page)).toContainText('Prefer small, verifiable changes.')
})
