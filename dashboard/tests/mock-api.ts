import { Page, Route } from '@playwright/test'
import type { SessionsResponse } from '../src/types'
import { DEFAULT_THEME } from '../src/theme/theme'

const fileResourcesPattern = /.*\/api\/files\/resources(?:\/.*)?$/
const themePattern = /.*\/api\/theme\/?$/
const launchPattern = /.*\/api\/launch\/?$/
const tmuxMousePattern = /.*\/api\/tmux\/mouse\/?$/
const tmuxSessionsPattern = /.*\/api\/tmux\/sessions\/?$/

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
  // are links to the right store.
  await mockBeadsProjectsRoute(page)

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

export async function mockBeadsProjectsRoute(page: Page, projectsResponse?: object) {
  await page.route('**/api/beads/projects**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(projectsResponse ?? mockBeadsProjects),
    })
  })
}

// Beads API mock routes - can be customized per test
export async function mockBeadsApiRoutes(page: Page, options?: {
  projectsResponse?: object
  workResponse?: object
  beadResponse?: object
}) {
  await mockBeadsProjectsRoute(page, options?.projectsResponse)

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

// Beads API error mock - simulates bd not installed
export async function mockBeadsApiError(page: Page) {
  await page.route('**/api/beads/projects**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockBeadsProjects),
    })
  })

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
