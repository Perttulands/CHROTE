import { Page, Route } from '@playwright/test'
import type { AgentEvent, SessionsResponse, TmuxSession } from '../src/types'
import { DEFAULT_THEME } from '../src/theme/theme'

const fileResourcesPattern = /.*\/api\/files\/resources(?:\/.*)?$/
const themePattern = /.*\/api\/theme\/?$/
const launchPattern = /.*\/api\/launch\/?$/
const tmuxMousePattern = /.*\/api\/tmux\/mouse\/?$/
const tmuxSessionsPattern = /.*\/api\/tmux\/sessions\/?$/
const agentEventSeenPattern = /.*\/api\/agent\/event\/seen\/?$/
const workspacesPattern = /.*\/api\/workspaces\/?(\?.*)?$/

// Mock beads data for testing
export const mockBeadsProjects = {
  success: true,
  timestamp: new Date().toISOString(),
  data: {
    projects: [
      { name: 'test-project', path: '/code/test-project', beadsPath: '/code/test-project/.beads', prefix: 'test' },
      { name: 'another-project', path: '/code/another-project', beadsPath: '/code/another-project/.beads', prefix: 'other' },
    ]
  }
}

const staleTimestamp = new Date(Date.now() - 40 * 86400000).toISOString()
const freshTimestamp = new Date(Date.now() - 86400000).toISOString()

export const mockBeadsWork = {
  success: true,
  timestamp: new Date().toISOString(),
  data: {
    prefix: 'test',
    projectPath: '/code/test-project',
    beads: [
      {
        id: 'test-ep1', title: 'One interaction language', status: 'open', type: 'epic', priority: 1,
        updated: freshTimestamp, acceptance: 'Every surface reads the same way', blocked: false,
      },
      {
        id: 'test-ep1.1', title: 'Fix login bug', status: 'open', type: 'bug', priority: 1,
        parent: 'test-ep1', updated: freshTimestamp, blocked: false,
      },
      {
        id: 'test-ep1.2', title: 'Add dark mode', status: 'in_progress', type: 'feature', priority: 2,
        parent: 'test-ep1', updated: freshTimestamp, blocked: false,
      },
      {
        id: 'test-ep1.3', title: 'Blocked by external API', status: 'open', type: 'task', priority: 2,
        parent: 'test-ep1', updated: staleTimestamp, blocked: true, blockedBy: ['test-ep1.2'],
      },
      {
        id: 'test-ep1.4', title: 'Completed feature', status: 'closed', type: 'feature', priority: 3,
        parent: 'test-ep1', updated: staleTimestamp, blocked: false,
      },
      // A second epic with a shape worth drawing: two chains that block
      // nothing of each other's, and one Bead that waits for both to finish.
      // The Flow view has three waves and two lanes to lay out from these.
      {
        id: 'test-ep2', title: 'Ship the reading room', status: 'open', type: 'epic', priority: 1,
        updated: freshTimestamp, acceptance: 'A page opens where it was found', blocked: false,
      },
      {
        id: 'test-ep2.1', title: 'Measure the shelves', status: 'open', type: 'task', priority: 2,
        parent: 'test-ep2', updated: freshTimestamp, blocked: false,
      },
      {
        id: 'test-ep2.2', title: 'Draw the shelves', status: 'in_progress', type: 'feature', priority: 2,
        parent: 'test-ep2', updated: freshTimestamp, blocked: false,
      },
      {
        id: 'test-ep2.3', title: 'Index the pages', status: 'open', type: 'task', priority: 2,
        parent: 'test-ep2', updated: freshTimestamp, blocked: true, blockedBy: ['test-ep2.1'],
      },
      {
        id: 'test-ep2.4', title: 'Search the index', status: 'open', type: 'feature', priority: 2,
        parent: 'test-ep2', updated: freshTimestamp, blocked: true, blockedBy: ['test-ep2.2'],
      },
      {
        id: 'test-ep2.5', title: 'Open a page from a search', status: 'open', type: 'feature', priority: 1,
        parent: 'test-ep2', updated: freshTimestamp, blocked: true, blockedBy: ['test-ep2.3', 'test-ep2.4'],
      },
      // Finished work keeps its place in the first wave: the server drops a
      // closed blocker from every blockedBy, so nothing waits on it any more.
      {
        id: 'test-ep2.6', title: 'Choose the shelf order', status: 'closed', type: 'decision', priority: 3,
        parent: 'test-ep2', updated: freshTimestamp, blocked: false,
      },
    ],
  }
}

export const mockBeadsDetail = {
  success: true,
  timestamp: new Date().toISOString(),
  data: {
    projectPath: '/code/test-project',
    bead: {
      id: 'test-ep1.1', title: 'Fix login bug', status: 'open', type: 'bug', priority: 1,
      updated: freshTimestamp, created: freshTimestamp,
      description: 'The login form drops the session. Follows test-ep1.2.',
      acceptance: 'A login survives a reload.',
      notes: 'Reported from a terminal.',
      parents: [{ id: 'test-ep1', title: 'One interaction language', status: 'open', type: 'epic', priority: 1 }],
      children: [],
      blockedBy: [],
      blocks: [{ id: 'test-ep1.3', title: 'Blocked by external API', status: 'open', type: 'task', priority: 2 }],
    },
  }
}

export const mockSystemStatus = {
  success: true,
  timestamp: new Date().toISOString(),
  data: {
    timestamp: new Date().toISOString(),
    host: {
      hostname: 'test-host',
      uptimeSeconds: 3600,
      load1: 1.2,
      load5: 1.1,
      load15: 1.0,
    },
    cpu: {
      cores: 4,
      totalTicks: 1000,
      idleTicks: 750,
    },
    memory: {
      totalBytes: 16 * 1024 * 1024 * 1024,
      freeBytes: 2 * 1024 * 1024 * 1024,
      availableBytes: 8 * 1024 * 1024 * 1024,
      usedBytes: 8 * 1024 * 1024 * 1024,
      usedPercent: 50,
      swapTotalBytes: 2 * 1024 * 1024 * 1024,
      swapUsedBytes: 128 * 1024 * 1024,
      swapUsedPercent: 6.25,
    },
    disks: [
      {
        mount: '/',
        totalBytes: 100 * 1024 * 1024 * 1024,
        availableBytes: 60 * 1024 * 1024 * 1024,
        usedBytes: 40 * 1024 * 1024 * 1024,
        usedPercent: 40,
      },
    ],
    network: [
      { name: 'eth0', rxBytes: 1000000, txBytes: 500000 },
    ],
    gpus: [
      { available: false, message: 'nvidia-smi unavailable' },
    ],
    warnings: [],
  },
}

/**
 * The host's workspaces as the server lists them: the folder a live session
 * runs in first, then the stores and repositories under the roots. One store
 * has nothing open, so the Beads rail has something to fold.
 */
export const mockWorkspaces = [
  { path: '/srv/chrote', sources: ['session', 'git'], sessions: ['gt-gastown-jack'], instructions: 2, lastActivity: freshTimestamp },
  {
    path: '/code/test-project',
    sources: ['beads', 'git', 'store'],
    sessions: [],
    beadsPrefix: 'test',
    openBeads: 4,
    instructions: 1,
    beadsCounts: {
      status: { open: 2, inProgress: 1, blocked: 1, closed: 9, deferred: 1 },
      type: { epic: 2, task: 4, bug: 1, feature: 5, decision: 1, chore: 1 },
    },
    beadsNewestUpdate: freshTimestamp,
  },
  {
    path: '/code/another-project',
    sources: ['git', 'store'],
    sessions: [],
    beadsPrefix: 'other',
    openBeads: 0,
    instructions: 0,
    beadsCounts: {
      status: { open: 0, inProgress: 0, blocked: 0, closed: 3, deferred: 0 },
      type: { epic: 0, task: 3, bug: 0, feature: 0, decision: 0, chore: 0 },
    },
    beadsNewestUpdate: freshTimestamp,
  },
  { path: '/home/operator/repos/VSK-Zone', sources: ['git'], sessions: [], instructions: 0 },
]

export async function mockWorkspacesRoute(page: Page, workspaces: object[] = mockWorkspaces) {
  await page.route(workspacesPattern, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(workspaces),
    })
  })
}

export const mockBeadsError = {
  success: false,
  timestamp: new Date().toISOString(),
  error: {
    code: 'BD_NOT_INSTALLED',
    message: 'bd command not found. Install modern Beads and ensure it is on CHROTE\'s PATH.'
  }
}

// Mock session data for testing
export const mockSessions = {
  sessions: [
    { name: 'hq-mayor', windows: 1, attached: false, group: 'hq' },
    { name: 'hq-deacon', windows: 1, attached: true, group: 'hq' },
    { name: 'main', windows: 2, attached: false, group: 'main' },
    { name: 'gt-gastown-jack', windows: 1, attached: false, group: 'gt-gastown' },
    { name: 'gt-gastown-joe', windows: 1, attached: false, group: 'gt-gastown' },
    { name: 'gt-gastown-max', windows: 1, attached: false, group: 'gt-gastown' },
    { name: 'gt-beads-lizzy', windows: 1, attached: false, group: 'gt-beads' },
    { name: 'gt-beads-darcy', windows: 1, attached: false, group: 'gt-beads' },
  ],
  grouped: {
    'hq': [
      { name: 'hq-mayor', windows: 1, attached: false, group: 'hq' },
      { name: 'hq-deacon', windows: 1, attached: true, group: 'hq' },
    ],
    'main': [
      { name: 'main', windows: 2, attached: false, group: 'main' },
    ],
    'gt-gastown': [
      { name: 'gt-gastown-jack', windows: 1, attached: false, group: 'gt-gastown' },
      { name: 'gt-gastown-joe', windows: 1, attached: false, group: 'gt-gastown' },
      { name: 'gt-gastown-max', windows: 1, attached: false, group: 'gt-gastown' },
    ],
    'gt-beads': [
      { name: 'gt-beads-lizzy', windows: 1, attached: false, group: 'gt-beads' },
      { name: 'gt-beads-darcy', windows: 1, attached: false, group: 'gt-beads' },
    ],
  },
  timestamp: new Date().toISOString(),
} satisfies SessionsResponse

function apiBody(data: object) {
  return JSON.stringify({
    success: true,
    timestamp: new Date().toISOString(),
    data,
  })
}

async function fulfillJson(route: Route, data: object, headers?: Record<string, string>) {
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    headers,
    body: apiBody(data),
  })
}

export async function mockFileApiRoutes(page: Page) {
  await page.route(fileResourcesPattern, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ isDir: true, items: [] }),
    })
  })

  await page.route('**/api/files/raw/**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'text/plain',
      body: 'mock file content',
    })
  })
}

const TTYD_OUTPUT = 0x30

/**
 * A ttyd stand-in on /terminal/ws. It answers the client's opening handshake
 * the way a freshly attached tmux session would: with output.
 */
export async function mockTerminalSocket(page: Page) {
  await page.routeWebSocket(url => url.pathname === '/terminal/ws', ws => {
    // The URL carries ttyd's `-a` fragments in order: viewing mode, session
    // name, then the optional Unix user. Index 0 is the mode, not the name.
    const sessionName = new URL(ws.url()).searchParams.getAll('arg')[1] ?? 'session'
    ws.onMessage(message => {
      const text = typeof message === 'string' ? message : message.toString('utf8')
      // Only the unprefixed JSON handshake spawns a pty; the rest is input,
      // resize and flow control.
      if (!text.startsWith('{')) return
      ws.send(Buffer.concat([
        Buffer.from([TTYD_OUTPUT]),
        Buffer.from(`mock terminal ${sessionName}\r\n$ `),
      ]))
    })
  })
}

export async function mockThemeApiRoute(page: Page) {
  // The dashboard reads its palette from the host once at startup. Serving the
  // embedded default keeps a journey's colours the ones the source declares.
  await page.route(themePattern, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(DEFAULT_THEME),
    })
  })
}

/**
 * What the launcher may offer in a mocked journey: two harnesses and two
 * folders. The catalogue carries one of each kind the contract names — a bare
 * boolean, a boolean with a short form, a value flag with possible values, and
 * a value flag with a free placeholder — so a journey can exercise all four.
 */
export const mockLaunchOptions = {
  harnesses: [
    {
      id: 'claude-code',
      label: 'Claude Code',
      binary: 'claude',
      defaultFlags: '--dangerously-skip-permissions',
      flags: [
        { name: '--continue', short: '-c', description: 'Continue the most recent conversation' },
        { name: '--verbose', description: 'Override verbose mode setting from config' },
        {
          name: '--model',
          short: '-m',
          value: '<model>',
          description: 'Model for the current session',
          values: ['sonnet', 'opus'],
        },
        { name: '--add-dir', value: '<directories...>', description: 'Additional directories to allow tool access to' },
      ],
    },
    { id: 'shell', label: 'Shell', binary: '', defaultFlags: '', flags: [] },
  ],
  folders: ['/srv/chrote', '~'],
}

export async function mockLaunchApiRoute(page: Page) {
  await page.route(launchPattern, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockLaunchOptions),
    })
  })
}

/**
 * The residents as a host names them: the Librarian and the Clerk share a
 * session that is in the mocked list with a client attached, so their columns
 * open live; the tender's session is not there, so its column offers Launch.
 */
export const mockResidents = [
  { tab: 'library', label: 'Librarian', session: 'hq-deacon', folder: '/corpus', beads: '/code/test-project' },
  { tab: 'agents', label: 'Tender', session: 'tender', folder: '/code/tender', beads: '/code/test-project' },
  { tab: 'beads', label: 'Clerk', session: 'hq-deacon', folder: '/code/clerk', beads: '/code/test-project' },
]

export async function mockResidentsApiRoute(page: Page, residents: object[] = mockResidents) {
  await page.route('**/api/residents', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(residents),
    })
  })
}

export async function mockApiRoutes(page: Page, options?: { sessionsResponse?: SessionsResponse }) {
  await mockTerminalSocket(page)
  await mockThemeApiRoute(page)
  await mockLaunchApiRoute(page)

  await page.route(tmuxMousePattern, async route => {
    const body = route.request().postDataJSON() as { enabled?: boolean } | null
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, mouse: body?.enabled ? 'on' : 'off', applied: 1, total: 1 }),
    })
  })

  await mockFileApiRoutes(page)
  await mockSystemStatusApiRoutes(page)
  // Every terminal asks which Beads projects exist, so that ids in its output
  // are links to the right store: the workspace list carries them, and the
  // projects route answers only for manual paths.
  await mockWorkspacesRoute(page)
  await mockBeadsProjectsRoute(page)
  // These tabs stay mounted after their first paint, including while another
  // tab is in front. Every browser journey therefore supplies their ordinary
  // read routes; a test can register a narrower response afterwards.
  await mockPersistentTabApiRoutes(page)

  await page.route(tmuxSessionsPattern, async route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { name?: string; cwd?: string; harness?: string } | null
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          success: true,
          name: body?.name ?? 'shell-test',
          session: body?.name ?? 'shell-test',
          cwd: body?.cwd ?? '~',
          harness: body?.harness ?? 'shell',
        }),
      })
      return
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(options?.sessionsResponse ?? mockSessions),
    })
  })

  // Mock WebSocket - just let it fail gracefully
  // The UI should handle disconnected state
}

/** A session list with one session carrying what its agent last reported. */
export function sessionsWithLastEvent(base: SessionsResponse, name: string, lastEvent: AgentEvent): SessionsResponse {
  const withEvent = (session: TmuxSession): TmuxSession => (session.name === name ? { ...session, lastEvent } : session)
  return {
    ...base,
    sessions: base.sessions.map(withEvent),
    grouped: Object.fromEntries(Object.entries(base.grouped).map(([group, list]) => [group, list.map(withEvent)])),
    timestamp: new Date().toISOString(),
  }
}

export interface AgentEventSeenRequest {
  session: string
  unixUser?: string
}

/**
 * The seen route, answering as the server does and keeping every body posted
 * to it, so a journey can prove that focusing the tile told the server.
 */
export async function mockAgentEventSeenRoute(page: Page): Promise<AgentEventSeenRequest[]> {
  const seen: AgentEventSeenRequest[] = []
  await page.route(agentEventSeenPattern, async route => {
    const body = route.request().postDataJSON() as AgentEventSeenRequest
    seen.push(body)
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true, session: body.session, unixUser: body.unixUser ?? '' }),
    })
  })
  return seen
}

export async function mockBeadsProjectsRoute(page: Page, projectsResponse?: object) {
  await page.route('**/api/beads/projects**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(projectsResponse ?? mockBeadsProjects),
    })
  })
}

export async function mockAgentContextApiRoutes(page: Page) {
  await page.route('**/api/agent/tender', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ session: 'tender', beads: '/code/test-project', folder: '/code/tender' }),
    })
  })

  await page.route('**/api/agent/context**', async route => {
    const url = new URL(route.request().url())
    const folder = url.searchParams.get('folder') ?? ''
    const harness = url.searchParams.get('harness') ?? 'claude-code'
    const user = url.searchParams.get('user') ?? ''
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        folder,
        harness,
        user,
        instructions: [{
          path: `${folder}/${harness === 'codex' ? 'AGENTS.md' : 'CLAUDE.md'}`,
          scope: 'project',
          kind: harness === 'codex' ? 'AGENTS.md' : 'CLAUDE.md',
          readable: true,
          size: 1200,
        }],
        skills: [],
        memories: [],
      }),
    })
  })
}

/** The ordinary reads made by the views that stay mounted across tab switches. */
export async function mockPersistentTabApiRoutes(page: Page) {
  await mockResidentsApiRoute(page)
  await mockBeadsApiRoutes(page)
  await mockLibraryApiRoutes(page)
  await mockAgentContextApiRoutes(page)
}

// Beads API mock routes - can be customized per test
export async function mockBeadsApiRoutes(page: Page, options?: {
  projectsResponse?: object
  workResponse?: object
  beadResponse?: object
}) {
  // mockApiRoutes already serves the project map, because every terminal asks
  // for it to link the ids in its output. Playwright answers with the most
  // recently registered handler, so registering the same map again here would
  // only shadow that one with an identical body. Register it only to override.
  if (options?.projectsResponse) await mockBeadsProjectsRoute(page, options.projectsResponse)

  // Each store answers for itself: the second project is empty, so "All" is a
  // sum of stores rather than the same rows twice.
  await page.route('**/api/beads/work**', async route => {
    const path = new URL(route.request().url()).searchParams.get('path')
    const empty = { success: true, timestamp: new Date().toISOString(), data: { prefix: 'other', projectPath: path, beads: [] } }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(options?.workResponse ?? (path === '/code/test-project' ? mockBeadsWork : empty)),
    })
  })

  await page.route('**/api/beads/issue**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(options?.beadResponse ?? mockBeadsDetail),
    })
  })
}

export async function mockSystemStatusApiRoutes(page: Page, onRequest?: () => void) {
  let sample = 0
  const buildStatusSample = (sequence: number, timestamp: string) => ({
    ...mockSystemStatus.data,
    timestamp,
    cpu: {
      ...mockSystemStatus.data.cpu,
      totalTicks: 1000 + sequence * 100,
      idleTicks: 750 + sequence * 55,
    },
    memory: {
      ...mockSystemStatus.data.memory,
      usedPercent: Math.min(92, 45 + (sequence % 24)),
    },
    host: {
      ...mockSystemStatus.data.host,
      load1: 0.8 + sequence / 10,
    },
    network: [
      { name: 'eth0', rxBytes: 1000000 + sequence * 1024, txBytes: 500000 + sequence * 512 },
    ],
  })

  await page.route('**/api/system/status', async route => {
    onRequest?.()
    sample += 1
    const timestamp = new Date().toISOString()
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        ...mockSystemStatus,
        timestamp,
        data: buildStatusSample(sample + 24, timestamp),
      }),
    })
  })

  await page.route('**/api/system/history', async route => {
    onRequest?.()
    const now = Date.now()
    const samples = Array.from({ length: 24 }, (_, index) => {
      const sequence = sample + index + 1
      const timestamp = new Date(now - (24 - index) * 60_000).toISOString()
      return buildStatusSample(sequence, timestamp)
    })
    await fulfillJson(route, { limit: 288, samples })
  })
}

// Beads API error mock - simulates bd not installed.
// The project map still answers, from the registration mockApiRoutes made:
// what fails is the two routes that carry the Beads themselves, which is the
// shape of a host that has the stores configured but no `bd` on its PATH.
export async function mockBeadsApiError(page: Page) {
  await page.route('**/api/beads/work**', async route => {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify(mockBeadsError),
    })
  })

  await page.route('**/api/beads/issue**', async route => {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify(mockBeadsError),
    })
  })
}

/**
 * A small corpus for the Library: two shelves, three pages, one history. The
 * routes are flat, as the server writes them, so a journey exercises the same
 * shapes the browser parses in production.
 */
const libraryChangedAt = new Date(Date.now() - 3 * 3600_000).toISOString()
const libraryLongAgo = new Date(Date.now() - 90 * 86_400_000).toISOString()

export const mockLibraryShelves = {
  root: '/corpus',
  librarianSession: 'hq-deacon',
  shelves: [
    { name: 'knowledge', path: 'knowledge', pages: 2 },
    { name: 'preferences', path: 'preferences', pages: 2 },
  ],
}

const mockLibraryPages: Record<string, object[]> = {
  knowledge: [
    { path: 'knowledge/testing.md', title: 'Test isolation', updated: libraryChangedAt, author: 'The Operator' },
    { path: 'knowledge/long.md', title: 'A note whose name runs well past the measure a label is drawn at', updated: libraryLongAgo, author: 'The Operator' },
  ],
  preferences: [
    { path: 'preferences/tools.md', title: 'Tool Preferences', updated: libraryChangedAt, author: 'The Operator' },
    { path: 'preferences/workflow.md', title: 'Workflow Preferences', updated: libraryChangedAt, author: 'The Operator' },
  ],
}

const mockLibraryPageContents: Record<string, { path: string; title: string; updated: string; author: string; content: string; history: object[] }> = {
  'preferences/workflow.md': {
    path: 'preferences/workflow.md',
    title: 'Workflow Preferences',
    updated: libraryChangedAt,
    author: 'The Operator',
    content: '# Workflow Preferences\n\nPrefer small, verifiable changes.\n',
    history: [
      { hash: 'c79783abc', time: libraryChangedAt, author: 'The Operator', message: 'Record a workflow preference' },
    ],
  },
  'preferences/tools.md': {
    path: 'preferences/tools.md',
    title: 'Tool Preferences',
    updated: libraryChangedAt,
    author: 'The Operator',
    content: '# Tool Preferences\n\nTools the operator reaches for.\n',
    history: [],
  },
}

/** The same corpus as the map reads it: three pages, two links, one shared tag. */
const mockLibraryGraph = {
  pages: [
    { path: 'knowledge/testing.md', shelf: 'knowledge', title: 'Test isolation', words: 60, updated: libraryChangedAt, candidate: false },
    { path: 'preferences/tools.md', shelf: 'preferences', title: 'Tool Preferences', words: 30, updated: libraryChangedAt, candidate: false },
    { path: 'preferences/workflow.md', shelf: 'preferences', title: 'Workflow Preferences', words: 200, updated: libraryChangedAt, candidate: false },
    // A name past the label measure, last touched long enough ago that any
    // window narrower than all leaves it behind.
    { path: 'knowledge/long.md', shelf: 'knowledge', title: 'A note whose name runs well past the measure a label is drawn at', words: 45, updated: libraryLongAgo, candidate: false },
  ],
  links: [['knowledge/testing.md', 'preferences/workflow.md'], ['preferences/workflow.md', 'preferences/tools.md']],
  tags: [['preferences/tools.md', 'preferences/workflow.md', 'tooling']],
}

/**
 * Enough arrivals to overflow the rail at a short window. The rail lists the
 * pages a commit touched, each once, so a fixture that overflows needs pages
 * to overflow with: the corpus pages first, then a run of one-off notes.
 */
const mockLibraryChanges = [
  {
    hash: 'c79783abc',
    time: libraryChangedAt,
    author: 'The Operator',
    message: 'Record a workflow preference',
    files: ['preferences/workflow.md', 'knowledge/testing.md'],
  },
  // The same page again, older: an arrival is listed once, at its newest.
  {
    hash: 'd41d8cd98',
    time: new Date(Date.now() - 30 * 3600_000).toISOString(),
    author: 'The Operator',
    message: 'Start the workflow preferences',
    files: ['preferences/workflow.md'],
  },
  ...Array.from({ length: 28 }, (_, index) => ({
    hash: `a${index.toString().padStart(6, '0')}`,
    time: new Date(Date.now() - (index + 4) * 3600_000).toISOString(),
    author: 'The Operator',
    message: `Curate the knowledge shelf, pass ${index + 1}`,
    files: [`knowledge/pass-${index + 1}.md`],
  })),
]

export async function mockLibraryApiRoutes(page: Page, options?: { shelves?: object }) {
  const flat = (route: Route, data: unknown) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })

  await page.route('**/api/library/shelves**', route => flat(route, options?.shelves ?? mockLibraryShelves))

  await page.route('**/api/library/changes**', route => flat(route, { changes: mockLibraryChanges }))

  await page.route('**/api/library/graph**', route => flat(route, mockLibraryGraph))

  await page.route('**/api/library/pages**', route => {
    const shelf = new URL(route.request().url()).searchParams.get('shelf') ?? ''
    return flat(route, { pages: mockLibraryPages[shelf] ?? [] })
  })

  await page.route('**/api/library/search**', route => {
    const query = (new URL(route.request().url()).searchParams.get('q') ?? '').toLowerCase()
    const hits = Object.values(mockLibraryPageContents)
      .filter(entry => entry.content.toLowerCase().includes(query) || entry.path.toLowerCase().includes(query))
      .map(entry => ({ path: entry.path, title: entry.title, line: 3, snippet: 'Prefer small, verifiable changes.' }))
    return flat(route, hits)
  })

  // The page route is matched exactly: Playwright tries the newest route
  // first, and a glob for `page` would swallow `pages` with it.
  await page.route(/\/api\/library\/page(\?|$)/, route => {
    if (route.request().method() === 'PUT') {
      const body = route.request().postDataJSON() as { summary?: string } | null
      return flat(route, {
        hash: 'newc0mm1t',
        time: new Date().toISOString(),
        author: 'The Operator',
        message: body?.summary ?? '',
      })
    }
    const path = new URL(route.request().url()).searchParams.get('path') ?? ''
    const found = mockLibraryPageContents[path]
    if (!found) {
      return route.fulfill({
        status: 404,
        contentType: 'application/json',
        body: JSON.stringify({ success: false, error: { code: 'NOT_FOUND', message: 'No such page' } }),
      })
    }
    return flat(route, found)
  })
}
