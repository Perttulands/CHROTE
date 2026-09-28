import { expect, test, type Locator, type Page } from './fixtures'
import { mockApiRoutes, mockSessions } from './mock-api'
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

/**
 * Enough sessions ahead of the target that its row starts below the fold of
 * the picker's scrolling list.
 */
const crowdedSessions = {
  ...mockSessions,
  sessions: [
    ...Array.from({ length: 24 }, (_, index) => ({
      name: `aa-filler-${String(index).padStart(2, '0')}`,
      windows: 1,
      attached: false,
      group: 'aa',
    })),
    ...mockSessions.sessions,
  ],
}

/** The row lies wholly inside the list's visible area. */
async function expectInView(row: Locator, list: Locator) {
  await expect.poll(async () => {
    const [r, l] = [await box(row), await box(list)]
    return r.y >= l.y - 1 && r.y + r.height <= l.y + l.height + 1
  }).toBe(true)
}

async function openWorkspace(page: Page, sends: SendRecord[]) {
  await page.setViewportSize({ width: 1400, height: 900 })
  await mockApiRoutes(page, { sessionsResponse: crowdedSessions })
  await mockSendRoutes(page, sends)
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
  await page.goto('/')
  await expect(page.locator('.terminal-window:visible')).toHaveCount(2)
}

test.describe('the Send to Session drawer', () => {
  // The drawer is a surface over the right edge of the workspace: it takes
  // nothing from the grid, so nothing moves under the pointer while a message
  // is written, and the tile it was opened from is still the target it offers.
  // The selected row is the only thing on screen that says where the message
  // goes, so it is in view without the operator scrolling for it; and a note
  // may run to several lines before Enter sends it.
  test('overlays the right edge at 380px, leaves the grid where it was, shows the target, and sends on Enter', async ({ page }) => {
    const sends: SendRecord[] = []
    await openWorkspace(page, sends)

    const tiles = page.locator('.terminal-window:visible')
    const before = [await box(tiles.nth(0)), await box(tiles.nth(1))]

    await page.getByRole('button', { name: `Send to session ${TARGET}` }).click()

    const drawer = page.getByRole('dialog', { name: 'Send to session' })
    await expect(drawer).toBeVisible()

    expect(await box(tiles.nth(0))).toEqual(before[0])
    expect(await box(tiles.nth(1))).toEqual(before[1])
    const drawerBox = await box(drawer)
    const content = await box(page.locator('.dashboard-content'))
    expect(Math.round(drawerBox.width)).toBe(380)
    expect(Math.round(drawerBox.x + drawerBox.width)).toBe(Math.round(content.x + content.width))

    const note = drawer.getByLabel('Message to send')
    await expect(note).toBeFocused()

    const row = drawer.getByRole('option', { name: new RegExp(TARGET) })
    const list = drawer.locator('.send-drawer-target-list')
    await expect(row).toHaveAttribute('aria-selected', 'true')
    await expectInView(row, list)

    // Scrolled away by hand, the row comes back when the search reshapes the
    // list; 'a' keeps it and most of the crowd, so the row still has to move.
    await list.evaluate(element => { element.scrollTop = 0 })
    await drawer.getByLabel('Search sessions').fill('a')
    await expectInView(row, list)

    await note.focus()
    await note.pressSequentially('status')
    await note.press('Shift+Enter')
    await note.pressSequentially('please')
    await expect(note).toHaveValue('status\nplease')
    expect(sends).toEqual([])
    await note.press('Enter')

    await expect(drawer).toHaveCount(0)
    // A multipart form carries every line break as CRLF, whatever the note held.
    expect(sends).toEqual([{ text: 'status\r\nplease', submit: 'true' }])
  })
})
