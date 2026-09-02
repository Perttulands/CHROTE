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

/** Bindings are written `user:name`, or bare when they name no user. */
const sessionName = (key: string) => (key.includes(':') ? key.slice(key.indexOf(':') + 1) : key)
const sessionUser = (key: string) => (key.includes(':') ? decodeURIComponent(key.slice(0, key.indexOf(':'))) : '')
const sessionKey = (name: string, unixUser: string) => (unixUser ? `${encodeURIComponent(unixUser)}:${name}` : name)

const session = (key: string) => {
  const unixUser = sessionUser(key)
  return { name: sessionName(key), windows: 1, attached: false, group: 'shell', ...(unixUser ? { unixUser } : {}) }
}

/** One configured user's tmux failing while the rest answer. */
interface PartialOutage {
  successfulUsers: string[]
  failedUsers: string[]
}

interface Harness {
  /** The session list the poll returns next, keyed the way bindings are. */
  live: { current: string[] }
  /** Set to make the poll answer partially, listing only the answering users. */
  partial: { current: PartialOutage | null }
  /** Live sockets by session key, so a test can end one the way tmux would. */
  sockets: Map<string, WebSocketRoute>
  /** How many times each session has been dialled. */
  dials: Map<string, number>
  created: string[]
}

interface WindowSpec {
  boundSessions: string[]
  activeSession: string | null
}

async function open(
  page: Page,
  boundSessions: string[],
  activeSession: string,
  options: { extraWindows?: WindowSpec[]; partial?: PartialOutage; live?: string[] } = {},
): Promise<Harness> {
  const harness: Harness = {
    live: { current: [...(options.live ?? boundSessions)] },
    partial: { current: options.partial ?? null },
    sockets: new Map(),
    dials: new Map(),
    created: [],
  }
  const windows: WindowSpec[] = [{ boundSessions, activeSession }, ...(options.extraWindows ?? [])]

  await mockApiRoutes(page)

  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    // The tile URL carries the viewing mode first, then the session name, then
    // the optional Unix user.
    const args = new URL(ws.url()).searchParams.getAll('arg')
    const name = sessionKey(args[1] ?? 'session', args[2] ?? '')
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
      const body = route.request().postDataJSON() as { name?: string; unixUser?: string } | null
      const key = sessionKey(body?.name ?? 'shell-test', body?.unixUser ?? '')
      harness.created.push(key)
      if (!harness.live.current.includes(key)) harness.live.current.push(key)
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ success: true, session: sessionName(key) }),
      })
      return
    }
    const outage = harness.partial.current
    // A partial response carries only the answering users' sessions, and names
    // both sides so a caller can tell which half of the list it may believe.
    const visible = outage
      ? harness.live.current.filter(key => outage.successfulUsers.includes(sessionUser(key)))
      : harness.live.current
    const sessions = visible.map(session)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        sessions,
        grouped: { shell: sessions },
        timestamp: new Date().toISOString(),
        ...(outage
          ? {
            partial: true,
            successfulUsers: outage.successfulUsers,
            failedUsers: outage.failedUsers,
            terminalUsers: [...outage.successfulUsers, ...outage.failedUsers],
            error: `${outage.failedUsers.join(', ')}: tmux socket unreachable`,
          }
          : {}),
      }),
    })
  })

  await page.addInitScript((state) => {
    localStorage.setItem('chrote-dashboard-state', JSON.stringify(state))
  }, {
    workspaces: {
      terminal1: {
        windowCount: windows.length,
        windows: windows.map((spec, index) => ({
          id: `terminal1-window-${index}`,
          boundSessions: spec.boundSessions,
          activeSession: spec.activeSession,
          colorIndex: index,
        })),
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

  test('a detached tile states itself in the middle of the frame it is preserving, in the empty-window button shape', async ({ page }) => {
    const harness = await open(page, ['doomed'], 'doomed')
    await expect(shownFrame(page)).toContainText('doomed output 1')

    harness.live.current = []
    await harness.sockets.get('doomed')!.close()
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'ended')

    const panel = tile(page).locator('.terminal-tile-detached')
    await expect(panel).toBeVisible()
    const panelBox = (await panel.boundingBox())!
    const bodyBox = (await windowBody(page).boundingBox())!

    // Centred on both axes rather than pinned along the bottom edge.
    expect(Math.abs((panelBox.y + panelBox.height / 2) - (bodyBox.y + bodyBox.height / 2))).toBeLessThan(4)
    expect(Math.abs((panelBox.x + panelBox.width / 2) - (bodyBox.x + bodyBox.width / 2))).toBeLessThan(4)
    // Compact: the last rendered frame is the thing this state exists to keep,
    // so the panel may not be what the tile mostly shows.
    expect(panelBox.height).toBeLessThan(bodyBox.height / 2)
    expect(panelBox.width).toBeLessThan(bodyBox.width)

    // The same button shape an empty window offers, and no other.
    await expect(tile(page).getByRole('button', { name: 'Restart' })).toHaveClass(/\btile-action-btn\b/)
    await expect(tile(page).getByRole('button', { name: 'Remove' })).toHaveClass(/\btile-action-btn\b/)
    await expect(tile(page).locator('.terminal-tile-action')).toHaveCount(0)
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

  test('a partial outage ends only the bindings it can speak for', async ({ page }) => {
    // 'alice' answers and no longer lists 'departed'. 'build' does not answer at
    // all, so its sessions are absent from the response for want of a reply,
    // not because they died — and a bare binding names no user to check.
    await open(page, ['alice:departed'], 'alice:departed', {
      live: [],
      partial: { successfulUsers: ['alice'], failedUsers: ['build'] },
      extraWindows: [
        { boundSessions: ['build:worker'], activeSession: 'build:worker' },
        { boundSessions: ['orphan'], activeSession: 'orphan' },
      ],
    })

    const body = (index: number) => windowBody(page).nth(index)

    // The session under the user that answered really is gone, and says so even
    // though the response as a whole failed for somebody else.
    await expect(body(0)).toHaveAttribute('data-tile-state', 'ended')
    await expect(tile(page).nth(0).getByText('departed ended. This frame shows its last output.')).toBeVisible()
    await expect(tile(page).nth(0).getByRole('button', { name: 'Restart' })).toBeVisible()

    // Neither of these is knowable from this response, so neither is claimed.
    await expect(body(1)).not.toHaveAttribute('data-tile-state', 'ended')
    await expect(tile(page).nth(1).getByRole('button', { name: 'Restart' })).toHaveCount(0)
    await expect(body(2)).not.toHaveAttribute('data-tile-state', 'ended')
    await expect(tile(page).nth(2).getByRole('button', { name: 'Restart' })).toHaveCount(0)

    // Several further partial polls land. The verdict does not drift either way.
    await page.waitForTimeout(1600)
    await expect(body(0)).toHaveAttribute('data-tile-state', 'ended')
    await expect(body(1)).not.toHaveAttribute('data-tile-state', 'ended')
    await expect(body(2)).not.toHaveAttribute('data-tile-state', 'ended')
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
