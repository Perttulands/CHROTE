import { test, expect, type Page } from './fixtures'
import type { WebSocketRoute } from '@playwright/test'
import { mockApiRoutes } from './mock-api'

/**
 * Tile states gate (bead chrote-jkzk.4, ADR-0017 decision 5).
 *
 * A binding is the operator's stated intent. Killing a process used to freeze
 * the frame and then, a poll later, unbind the session and slide whatever else
 * was bound into the frame the operator was reading. A tile now changes only
 * when the operator changes it, and says honestly what became of its session.
 */

const TTYD_OUTPUT = 0x30

const session = (name: string) => ({ name, windows: 1, attached: false, group: 'shell' })

interface Harness {
  /** The session list the poll returns next. */
  live: { current: string[] }
  /** Live sockets by session name, so a test can end one the way tmux would. */
  sockets: Map<string, WebSocketRoute>
  /** How many times each session has been dialled. */
  dials: Map<string, number>
  created: string[]
}

async function open(page: Page, boundSessions: string[], activeSession: string): Promise<Harness> {
  const harness: Harness = {
    live: { current: [...boundSessions] },
    sockets: new Map(),
    dials: new Map(),
    created: [],
  }

  await mockApiRoutes(page)

  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    // The tile URL carries the viewing mode first, then the session name.
    const name = new URL(ws.url()).searchParams.getAll('arg')[1] ?? 'session'
    harness.sockets.set(name, ws)
    const dial = (harness.dials.get(name) ?? 0) + 1
    harness.dials.set(name, dial)
    // Dialling a session tmux no longer has drops straight away, as the real
    // launch path does. This is what a reloaded ended binding meets.
    if (!harness.live.current.includes(name)) {
      void ws.close()
      return
    }
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      if (!text.startsWith('{')) return
      ws.send(Buffer.concat([Buffer.from([TTYD_OUTPUT]), Buffer.from(`${name} output ${dial}`)]))
    })
  })

  await page.route(/.*\/api\/tmux\/sessions\/?$/, async route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { name?: string } | null
      const name = body?.name ?? 'shell-test'
      harness.created.push(name)
      if (!harness.live.current.includes(name)) harness.live.current.push(name)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, session: name }),
      })
      return
    }
    const sessions = harness.live.current.map(session)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ sessions, grouped: { shell: sessions }, timestamp: new Date().toISOString() }),
    })
  })

  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, {
    workspaces: {
      terminal1: {
        windowCount: 1,
        windows: [{ id: 'terminal1-window-0', boundSessions, activeSession, colorIndex: 0 }],
      },
      terminal2: { windowCount: 1, windows: [{ id: 'terminal2-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }] },
      terminal3: { windowCount: 1, windows: [{ id: 'terminal3-window-0', boundSessions: [], activeSession: null, colorIndex: 0 }] },
    },
    sidebarCollapsed: false,
    settings: { theme: 'dark', fontSize: 14, autoRefreshInterval: 500 },
  })
  await page.goto('/')
  return harness
}

const tile = (page: Page) => page.locator('.terminal-grid[data-workspace="terminal1"] .terminal-window')
/** Only the binding a window is showing has a visible surface. */
const shownFrame = (page: Page) => tile(page).locator('.terminal-surface-host:visible .xterm-rows')
const activeTag = (page: Page) => tile(page).locator('.session-tag.active .tag-name')
const windowBody = (page: Page) => tile(page).locator('.terminal-window-body')

test.describe('Tile states', () => {
  test('a session that dies while viewed keeps its tile, its final output, and its place', async ({ page }) => {
    const harness = await open(page, ['doomed', 'survivor'], 'doomed')

    await expect(shownFrame(page)).toContainText('doomed output 1')
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')

    // tmux loses the session: the connection dies at once, the poll catches up
    // a beat later. Neither may move the tile.
    harness.live.current = ['survivor']
    await harness.sockets.get('doomed')!.close()

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'ended')
    await expect(tile(page).getByText('doomed ended. This frame shows its last output.')).toBeVisible()

    // The frame the operator was reading is still the frame on screen, still
    // carrying what died in it, and no other bound session took it over.
    await expect(shownFrame(page)).toContainText('doomed output 1')
    await expect(shownFrame(page)).not.toContainText('survivor')
    await expect(activeTag(page)).toHaveText('doomed')
    await expect(tile(page).locator('.session-tag .tag-name')).toHaveText(['doomed', 'survivor'])
    expect(harness.dials.get('survivor')).toBe(1)

    // Several further polls land, all of them without the session. Still there.
    await page.waitForTimeout(1600)
    await expect(activeTag(page)).toHaveText('doomed')
    await expect(shownFrame(page)).toContainText('doomed output 1')
    await expect(tile(page).getByRole('button', { name: 'Restart' })).toBeVisible()
    await expect(tile(page).getByRole('button', { name: 'Remove' })).toBeVisible()
    await expect(tile(page).getByRole('button', { name: 'Reclaim' })).toHaveCount(0)
  })

  test('an ended binding survives a reload, and Restart brings the session back into the same tile', async ({ page }) => {
    const harness = await open(page, ['doomed'], 'doomed')
    await expect(shownFrame(page)).toContainText('doomed output 1')

    harness.live.current = []
    await harness.sockets.get('doomed')!.close()
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'ended')

    await page.reload()
    await expect(activeTag(page)).toHaveText('doomed')
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'ended')

    await tile(page).getByRole('button', { name: 'Restart' }).click()

    // Third dial: the first one before the session died, the second refused
    // after the reload, this one on the session Restart recreated.
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')
    await expect(shownFrame(page)).toContainText('doomed output 3')
    await expect(activeTag(page)).toHaveText('doomed')
    expect(harness.created).toEqual(['doomed'])
  })

  test('a tile whose session was claimed elsewhere offers Reclaim, and takes it back', async ({ page }) => {
    const harness = await open(page, ['claimed'], 'claimed')
    await expect(shownFrame(page)).toContainText('claimed output 1')

    // Another client attached with -d: our client is gone, the session is not.
    await harness.sockets.get('claimed')!.close()

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'takenOver')
    await expect(tile(page).getByText('claimed is attached elsewhere. This frame shows its last output.')).toBeVisible()
    await expect(shownFrame(page)).toContainText('claimed output 1')
    await expect(tile(page).getByRole('button', { name: 'Restart' })).toHaveCount(0)

    await tile(page).getByRole('button', { name: 'Reclaim' }).click()

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')
    await expect(shownFrame(page)).toContainText('claimed output 2')
    expect(harness.created).toEqual([])
  })
})
