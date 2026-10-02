import { readFileSync } from 'node:fs'
import { expect, test, type Locator, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'

/**
 * Pictures and video in the Files panel's pop-out (bead: chrote-jtvh).
 *
 * The fit is measured against the window's real room and the video is decoded
 * by the real browser, which is why this is a journey: jsdom has neither
 * layout nor a media pipeline. A tall picture that used to run off the bottom
 * is shown whole, and a short mp4 plays inside the same window.
 */

const TALL = readFileSync(new URL('./media/tall.png', import.meta.url))
const CLIP = readFileSync(new URL('./media/clip.mp4', import.meta.url))

async function mockMediaFiles(page: Page) {
  // The server answers byte ranges (http.ServeContent), which is what makes a
  // video seekable; the stand-in does the same.
  await page.route('**/api/files/raw/**', route => {
    const path = new URL(route.request().url()).pathname
    if (!path.endsWith('.mp4')) return route.fulfill({ status: 200, contentType: 'image/png', body: TALL })
    const range = /bytes=(\d+)-(\d*)/.exec(route.request().headers().range ?? '')
    if (!range) {
      return route.fulfill({ status: 200, contentType: 'video/mp4', headers: { 'Accept-Ranges': 'bytes' }, body: CLIP })
    }
    const start = Number(range[1])
    const end = range[2] ? Math.min(Number(range[2]), CLIP.length - 1) : CLIP.length - 1
    return route.fulfill({
      status: 206,
      contentType: 'video/mp4',
      headers: { 'Accept-Ranges': 'bytes', 'Content-Range': `bytes ${start}-${end}/${CLIP.length}` },
      body: CLIP.subarray(start, end + 1),
    })
  })
  await page.route('**/api/files/diff*', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ path: '/', repository: '', diff: '', truncated: false }),
  }))
  await page.route(/\/api\/files\/resources\/?$/, route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      isDir: true,
      items: [
        { name: 'tall.png', size: TALL.length, modified: '2026-10-02T00:00:00Z', isDir: false, type: 'image/png' },
        { name: 'clip.mp4', size: CLIP.length, modified: '2026-10-02T00:00:00Z', isDir: false, type: 'video/mp4' },
      ],
    }),
  }))
}

async function box(locator: Locator) {
  const value = await locator.boundingBox()
  if (!value) throw new Error('expected a rendered bounding box')
  return value
}

/** The media is drawn whole inside the viewer's body, with nothing to scroll to. */
async function expectWhollyInside(media: Locator, frame: Locator) {
  const inner = await box(media)
  const outer = await box(frame)
  expect(inner.x).toBeGreaterThanOrEqual(outer.x - 0.5)
  expect(inner.y).toBeGreaterThanOrEqual(outer.y - 0.5)
  expect(inner.x + inner.width).toBeLessThanOrEqual(outer.x + outer.width + 0.5)
  expect(inner.y + inner.height).toBeLessThanOrEqual(outer.y + outer.height + 0.5)
  expect(await frame.evaluate(element => element.scrollHeight <= element.clientHeight)).toBe(true)
}

test('the Files panel fits a tall picture to its window and plays an mp4 inline', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 720 })
  await mockApiRoutes(page)
  await mockMediaFiles(page)
  await page.goto('/')
  await page.getByRole('button', { name: 'Files sidecar', exact: true }).click()

  const panel = page.locator('.terminal-files-panel')
  await panel.getByRole('treeitem', { name: /tall\.png/ }).click()
  const pictureWindow = page.getByRole('dialog', { name: 'File tall.png' })
  const picture = pictureWindow.getByRole('img', { name: 'tall.png' })
  await expect(pictureWindow.locator('.files-panel-note')).toHaveText('200 × 2000')
  // Two thousand pixels tall in a window under seven hundred: scaled down at
  // its own ratio, whole.
  const drawn = await box(picture)
  expect(drawn.height).toBeLessThan(720)
  expect(Math.abs(drawn.width / drawn.height - 0.1)).toBeLessThan(0.01)
  await expectWhollyInside(picture, pictureWindow.locator('[data-ui="files.viewer"]'))

  await panel.getByRole('treeitem', { name: /clip\.mp4/ }).click()
  const videoWindow = page.getByRole('dialog', { name: 'File clip.mp4' })
  const video = videoWindow.locator('video')
  await expect(videoWindow.locator('.files-panel-note')).toHaveText('1280 × 720')
  // Wider than the window: down to its width, at sixteen to nine.
  const frame = await box(video)
  expect(frame.width).toBeLessThan(1280)
  expect(Math.abs(frame.width / frame.height - 16 / 9)).toBeLessThan(0.02)
  await expectWhollyInside(video, videoWindow.locator('[data-ui="files.viewer"]'))

  // It plays, and it seeks.
  await video.evaluate(async (element: HTMLVideoElement) => {
    element.muted = true
    await element.play()
  })
  await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.currentTime)).toBeGreaterThan(0)
  await video.evaluate((element: HTMLVideoElement) => {
    element.pause()
    element.currentTime = 0.6
  })
  await expect.poll(() => video.evaluate((element: HTMLVideoElement) => element.seeking)).toBe(false)
  expect(await video.evaluate((element: HTMLVideoElement) => element.currentTime)).toBeCloseTo(0.6, 1)
})
