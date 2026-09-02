import { test, expect, allowBrowserConsoleMessage, type Page } from './fixtures'
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

const session = (key: string, heldElsewhere: readonly string[] = []) => {
  const unixUser = sessionUser(key)
  const held = heldElsewhere.includes(key)
  return {
    name: sessionName(key),
    windows: 1,
    attached: held,
    group: 'shell',
    ...(unixUser ? { unixUser } : {}),
    // What tmux reports for a client CHROTE did not create, such as an SSH login.
    ...(held ? { foreignClients: ['/dev/pts/12'] } : {}),
  }
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
  /** Set to make the poll fail outright, the way an unreachable host does. */
  failing: { current: boolean }
  /** Sessions the poll reports a client CHROTE did not create attached to. */
  heldElsewhere: { current: string[] }
  /** Live sockets by session key, so a test can end one the way tmux would. */
  sockets: Map<string, WebSocketRoute>
  /** How many times each session has been dialled. */
  dials: Map<string, number>
  /** How many times each session's connection asked for the sizing seat. */
  claims: Map<string, number>
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
  options: { extraWindows?: WindowSpec[]; partial?: PartialOutage; live?: string[]; heldElsewhere?: string[] } = {},
): Promise<Harness> {
  const harness: Harness = {
    live: { current: [...(options.live ?? boundSessions)] },
    partial: { current: options.partial ?? null },
    failing: { current: false },
    heldElsewhere: { current: [...(options.heldElsewhere ?? [])] },
    sockets: new Map(),
    dials: new Map(),
    claims: new Map(),
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
      // `4` is the claim frame: this connection asking to become the session's
      // one sizing client.
      if (text === '4') {
        harness.claims.set(name, (harness.claims.get(name) ?? 0) + 1)
        return
      }
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
    if (harness.failing.current) {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'tmux socket unreachable' }),
      })
      return
    }
    const outage = harness.partial.current
    // A partial response carries only the answering users' sessions, and names
    // both sides so a caller can tell which half of the list it may believe.
    const visible = outage
      ? harness.live.current.filter(key => outage.successfulUsers.includes(sessionUser(key)))
      : harness.live.current
    const sessions = visible.map(key => session(key, harness.heldElsewhere.current))
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

/** The operator leaves the dashboard and comes back to it. */
async function returnToTab(page: Page) {
  await page.evaluate(() => {
    const setVisibility = (state: string) => {
      Object.defineProperty(document, 'visibilityState', { value: state, configurable: true })
      document.dispatchEvent(new Event('visibilitychange'))
    }
    setVisibility('hidden')
    setVisibility('visible')
  })
}

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
    await expect(tile(page).getByRole('button', { name: 'Claim', exact: true })).toHaveCount(0)
  })

  test('a poll that fails does not take back an Ended verdict or offer Claim on a session that is gone', async ({ page }) => {
    allowBrowserConsoleMessage('Failed to load resource: the server responded with a status of 500')
    const harness = await open(page, ['doomed'], 'doomed')
    await expect(shownFrame(page)).toContainText('doomed output 1')

    harness.live.current = []
    await harness.sockets.get('doomed')!.close()
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'ended')

    // The poll now fails outright. That is the absence of news, not news that
    // a dead session came back, so the verdict CHROTE already reached stands.
    harness.failing.current = true
    await page.waitForTimeout(1600)

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'ended')
    await expect(tile(page).getByText('doomed ended. This frame shows its last output.')).toBeVisible()
    await expect(tile(page).getByRole('button', { name: 'Restart' })).toBeVisible()
    await expect(tile(page).getByRole('button', { name: 'Remove' })).toBeVisible()
    // Claim would dial a session tmux does not have and close at once.
    await expect(tile(page).getByRole('button', { name: 'Claim', exact: true })).toHaveCount(0)

    // Held one poll deep, not forever: a poll that answers again replaces the
    // verdict, so a session back under the same name is not stuck as Ended.
    harness.failing.current = false
    harness.live.current = ['doomed']
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')
    await expect(tile(page).getByText('doomed ended. This frame shows its last output.')).toHaveCount(0)
  })

  test('a partial outage does not take back an Ended verdict under the user that failed', async ({ page }) => {
    const harness = await open(page, ['build:doomed'], 'build:doomed')
    await expect(shownFrame(page)).toContainText('build:doomed output 1')

    harness.live.current = []
    await harness.sockets.get('build:doomed')!.close()
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'ended')

    // 'build' now stops answering while 'alice' still does. The response says
    // nothing about this binding, so it is in no position to overturn what the
    // last response that could speak for it already settled.
    harness.partial.current = { successfulUsers: ['alice'], failedUsers: ['build'] }
    await page.waitForTimeout(1600)

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'ended')
    await expect(tile(page).getByText('doomed ended. This frame shows its last output.')).toBeVisible()
    await expect(tile(page).getByRole('button', { name: 'Restart' })).toBeVisible()
    // Claim would dial a session tmux does not have and close at once.
    await expect(tile(page).getByRole('button', { name: 'Claim', exact: true })).toHaveCount(0)

    // Still only held: the socket recovering, with the session back under the
    // same name, is news the verdict gives way to.
    harness.partial.current = null
    harness.live.current = ['build:doomed']
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')
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

    // Reloading aborts whatever the page had in flight, and this tile polls
    // sessions twice a second, so the reload lands mid-poll often enough to
    // matter (3 failures in 120 runs at main 2c316e6e, always here, never on an
    // assertion). The rejected fetch is the reload, not a poll regression: the
    // page logs it on its way out, where no operator can see it.
    allowBrowserConsoleMessage('Failed to fetch sessions: TypeError: Failed to fetch')
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

  // The operator's report, 2026-09-02: twenty tiles saying the session was
  // attached elsewhere minutes after a deploy, with one client on the whole
  // socket, and twenty Reclaim clicks to recover. A restart kills every pty, so
  // every tile's connection goes while every session lives (ADR-0013).
  test('a tile whose connection went with a restart reconnects itself instead of claiming a takeover', async ({ page }) => {
    const harness = await open(page, ['restarted'], 'restarted')
    await expect(shownFrame(page)).toContainText('restarted output 1')

    // The server process died: the socket goes with no close frame at all.
    await harness.sockets.get('restarted')!.close({ code: 1006 })

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'lost')
    await expect(tile(page).getByText(/attached elsewhere/)).toHaveCount(0)
    await expect(tile(page).getByRole('button', { name: 'Claim', exact: true })).toHaveCount(0)

    await returnToTab(page)

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')
    await expect(shownFrame(page)).toContainText('restarted output 2')
    expect(harness.dials.get('restarted')).toBe(2)
    expect(harness.created).toEqual([])
  })

  // This used to be held back, because dialling attached with -d and would have
  // thrown the SSH client out. Nothing attaches with -d now (ADR-0017 decision
  // 1), so the dial joins them without taking their client or their size.
  test('a tile whose connection went dials again even when another client is attached', async ({ page }) => {
    const harness = await open(page, ['ssh-held'], 'ssh-held', { heldElsewhere: ['ssh-held'] })
    await expect(shownFrame(page)).toContainText('ssh-held output 1')

    await harness.sockets.get('ssh-held')!.close({ code: 1006 })

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'lost')
    await returnToTab(page)

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')
    await expect(shownFrame(page)).toContainText('ssh-held output 2')
    expect(harness.dials.get('ssh-held')).toBe(2)
    // Watching alongside them is not claiming the size from them.
    expect(harness.claims.get('ssh-held')).toBeUndefined()
  })

  test('a tile another client detached offers Claim, which dials again and takes the size', async ({ page }) => {
    const harness = await open(page, ['claimed'], 'claimed')
    await expect(shownFrame(page)).toContainText('claimed output 1')

    // A client outside CHROTE attached with -d: our client is gone, the session
    // is not. The pty hangs up, so the server closes the way it does for any
    // terminal that ended — which is what tells this apart from a lost one.
    await harness.sockets.get('claimed')!.close({ code: 1000, reason: 'terminal ended' })

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'takenOver')
    await expect(tile(page).getByText('claimed was detached by another client. This frame shows its last output.')).toBeVisible()
    await expect(shownFrame(page)).toContainText('claimed output 1')
    await expect(tile(page).getByRole('button', { name: 'Restart' })).toHaveCount(0)

    await tile(page).getByRole('button', { name: 'Claim', exact: true }).click()

    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')
    await expect(shownFrame(page)).toContainText('claimed output 2')
    // The dial alone would leave the session at whatever size the other client
    // set; the claim is what brings it back to this device.
    await expect.poll(() => harness.claims.get('claimed')).toBe(1)
    expect(harness.created).toEqual([])
  })

  test('claiming a session that is already on screen takes the size without redialling', async ({ page }) => {
    const harness = await open(page, ['shared'], 'shared')
    await expect(shownFrame(page)).toContainText('shared output 1')

    await tile(page).getByRole('button', { name: 'Session shared', exact: true }).click({ button: 'right' })
    await page.getByRole('menuitem', { name: /Claim session/ }).click()

    await expect.poll(() => harness.claims.get('shared')).toBe(1)
    expect(harness.dials.get('shared')).toBe(1)
    await expect(windowBody(page)).toHaveAttribute('data-tile-state', 'live')
  })
})
