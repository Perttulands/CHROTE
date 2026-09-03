import { expect, test, type Locator, type Page } from './fixtures'
import { mockApiRoutes } from './mock-api'
import { setWorkspaceState } from './helpers'

const TARGET = 'gt-gastown-jack'

interface SendRecord {
  text: string
  submit: string
}

async function box(locator: Locator) {
  const value = await locator.boundingBox()
  if (!value) throw new Error('expected a rendered bounding box')
  return value
}

/** The one multipart field, read out of the body the browser actually posted. */
function field(body: string, name: string): string {
  const match = new RegExp(`name="${name}"\\r?\\n\\r?\\n([\\s\\S]*?)\\r?\\n--`).exec(body)
  return match ? match[1] : ''
}

/**
 * A tmux that owns one pane and accepts what is pasted into it. The send reply
 * echoes the submission the drawer asked for, because the client refuses a
 * reply that claims something else.
 */
async function mockSendRoutes(page: Page, sends: SendRecord[]) {
  await page.route('**/api/tmux/sessions/*/panes', async route => {
    const session = decodeURIComponent(new URL(route.request().url()).pathname.split('/')[4])
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        success: true,
        session,
        unixUser: '',
        panes: [{
          sessionId: '$1',
          pane: '%1',
          panePid: '4242',
          serverPid: '9001',
          windowId: '@1',
          windowName: 'main',
          currentPath: '/srv/chrote',
          currentCommand: 'bash',
          active: true,
        }],
      }),
    })
  })

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

async function openWorkspace(page: Page, sends: SendRecord[]) {
  await page.setViewportSize({ width: 1400, height: 900 })
  await mockApiRoutes(page)
  await mockSendRoutes(page, sends)
  await page.goto('/')
  await setWorkspaceState(page, {
    workspaces: {
      terminal1: {
        windowCount: 2,
        windows: [
          { id: 'terminal1-window-0', boundSessions: [TARGET], activeSession: TARGET, colorIndex: 0 },
          { id: 'terminal1-window-1', boundSessions: ['gt-beads-lizzy'], activeSession: 'gt-beads-lizzy', colorIndex: 1 },
        ],
      },
      terminal2: { windowCount: 2, windows: [] },
    },
  })
  await page.reload()
  await expect(page.locator('.terminal-window:visible')).toHaveCount(2)
}

test.describe('the Send to Session drawer', () => {
  // One page for both layouts the drawer has: it takes a column from the grid
  // when the workspace has one to give, and overlays the grid when it does not.
  test('takes its column from the grid with room, and overlays the grid without', async ({ page }) => {
    const sends: SendRecord[] = []
    await openWorkspace(page, sends)

    const tile = page.locator('.terminal-window:visible').first()
    const before = await box(tile)

    await page.getByRole('button', { name: `Send to session ${TARGET}` }).click()

    const drawer = page.getByRole('dialog', { name: 'Send to session' })
    await expect(drawer).toBeVisible()

    // The grid gave the drawer its room rather than being covered by it: the
    // tile is narrower, still on screen, and ends before the drawer begins.
    const after = await box(tile)
    const drawerBox = await box(drawer)
    expect(after.width).toBeLessThan(before.width)
    await expect(tile).toBeVisible()
    expect(after.x + after.width).toBeLessThanOrEqual(drawerBox.x + 1)
    expect(Math.round(drawerBox.width)).toBe(380)

    // The tile it was opened from is the target it offers.
    await expect(drawer.getByRole('option', { name: new RegExp(TARGET) }))
      .toHaveAttribute('aria-selected', 'true')

    const note = drawer.getByLabel('Message to send')
    await expect(note).toBeFocused()
    await note.fill('status please')
    await note.press('Enter')

    await expect(drawer).toHaveCount(0)
    expect(sends).toEqual([{ text: 'status please', submit: 'true' }])
    await expect(page.locator('.status-line')).toContainText(`Pasted to '${TARGET}'`)

    // Narrow the workspace until there is no column to give, and the same
    // drawer overlays the grid instead of docking beside it.
    await page.setViewportSize({ width: 800, height: 900 })
    await page.getByRole('button', { name: `Send to session ${TARGET}` }).click()
    await expect(drawer).toBeVisible()

    expect(await page.locator('.send-drawer-gutter').count()).toBe(0)
    const overlaid = await box(drawer)
    expect(Math.round(overlaid.width)).toBe(400)
  })
})

