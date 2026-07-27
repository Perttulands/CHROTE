import { Page, Route } from '@playwright/test'
import type { SessionsResponse } from '../src/types'

const fileResourcesPattern = /.*\/api\/files\/resources(?:\/.*)?$/
const tmuxAppearancePattern = /.*\/api\/tmux\/appearance\/?$/
const tmuxMousePattern = /.*\/api\/tmux\/mouse\/?$/
const tmuxSessionsPattern = /.*\/api\/tmux\/sessions\/?$/

// Mock beads data for testing
export const mockBeadsProjects = {
  success: true,
  timestamp: new Date().toISOString(),
  data: {
    projects: [
      { name: 'test-project', path: '/code/test-project', beadsPath: '/code/test-project/.beads' },
      { name: 'another-project', path: '/code/another-project', beadsPath: '/code/another-project/.beads' },
    ]
  }
}

export const mockBeadsIssues = {
  success: true,
  timestamp: new Date().toISOString(),
  data: {
    issues: [
      { id: 'ISSUE-001', title: 'Fix login bug', status: 'open', priority: 1, type: 'bug' },
      { id: 'ISSUE-002', title: 'Add dark mode', status: 'in_progress', priority: 2, type: 'feature' },
      { id: 'ISSUE-003', title: 'Update dependencies', status: 'ready', priority: 3, type: 'chore' },
      { id: 'ISSUE-004', title: 'Blocked by external API', status: 'blocked', priority: 1, type: 'bug', dependencies: ['ISSUE-001'] },
      { id: 'ISSUE-005', title: 'Completed feature', status: 'closed', priority: 2, type: 'feature' },
    ],
    totalCount: 5,
    projectPath: '/code/test-project'
  }
}

export const mockBeadsTriage = {
  success: true,
  timestamp: new Date().toISOString(),
  data: {
    recommendations: [
      { issueId: 'ISSUE-001', rank: 1, reasoning: 'High priority bug blocking users', estimatedImpact: 'high' },
      { issueId: 'ISSUE-002', rank: 2, reasoning: 'User requested feature', estimatedImpact: 'medium' },
    ],
    quickWins: ['ISSUE-003'],
    blockers: ['ISSUE-004'],
  }
}

export const mockBeadsInsights = {
  success: true,
  timestamp: new Date().toISOString(),
  data: {
    issueCount: 5,
    openCount: 1,
    blockedCount: 1,
    closedCount: 1,
    byStatus: { open: 1, in_progress: 1, ready: 1, blocked: 1, closed: 1 },
    byType: { bug: 2, feature: 2, chore: 1 },
    health: {
      score: 75,
      risks: ['1 blocked issue needs attention'],
      warnings: ['Consider prioritizing quick wins'],
    },
    metrics: {
      density: 0.2,
      cycles: [],
      criticalPath: ['ISSUE-001', 'ISSUE-004'],
    }
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
    code: 'BV_NOT_INSTALLED',
    message: 'bv command not found. Install beads_viewer: go install github.com/Dicklesworthstone/beads_viewer@latest'
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

export const mockFormationsBoard = {
  id: 'board-playwright-smoke',
  slug: 'playwright-smoke',
  title: 'Playwright smoke board',
  rev: 3,
  etag: '"board-playwright-smoke-rev-3"',
  missions: [
    {
      id: 'mission-smoke',
      title: 'Smoke mission',
      goal: 'Exercise mocked Formations dashboard flow',
      beadId: 'home-28ps.2',
    },
  ],
  formations: [
    {
      id: 'formation-review',
      type: 'orchestrated',
      title: 'Review formation',
      brief: {
        goal: 'Review the Playwright stack setup',
        beadId: 'home-28ps.2',
        files: ['dashboard/tests/formations/formations-smoke.spec.ts'],
        links: [],
      },
      inputs: [{ id: 'in', label: 'input' }],
      outputs: [{ id: 'out', label: 'output' }],
      slots: [
        {
          id: 'slot-controller',
          label: 'Controller',
          controller: true,
          agentId: 'codex',
          harness: 'openai-codex',
        },
        {
          id: 'slot-reviewer',
          label: 'Reviewer',
          controller: false,
          agentId: 'claude',
          harness: 'claude-code',
        },
      ],
      verification: {
        id: 'verify-stack',
        kinds: ['human'],
        criterion: 'Playwright can mount mocked Formations data without live sessions',
        onFail: 'block',
      },
    },
  ],
  gates: [
    {
      id: 'gate-review',
      title: 'Review gate',
      kinds: ['human'],
      criterion: 'Reviewer confirms stack setup evidence is recorded',
    },
  ],
  connections: [
    {
      id: 'conn-review-gate',
      from: 'formation-review:out',
      to: 'gate-review:in',
    },
  ],
}

export const mockCodeGateProfiles = [
  {
    profileId: 'output_absent',
    profileVersion: '1',
    displayName: 'Output excludes value',
    parameterName: 'value',
    parameterLabel: 'Forbidden text',
  },
  {
    profileId: 'output_contains',
    profileVersion: '1',
    displayName: 'Output contains value',
    parameterName: 'value',
    parameterLabel: 'Required text',
  },
]

export const mockFormationsLayout = {
  boardId: mockFormationsBoard.id,
  boardRev: mockFormationsBoard.rev,
  etag: '"layout-playwright-smoke-rev-3"',
  nodes: [
    { id: 'mission-smoke', x: 80, y: 90 },
    { id: 'formation-review', x: 360, y: 120 },
    { id: 'gate-review', x: 700, y: 130 },
  ],
}

export const mockFormationsAgents = [
  {
    id: 'codex',
    displayName: 'Codex',
    harnessDefault: 'openai-codex',
    assignable: true,
    liveness: 'offline',
  },
  {
    id: 'claude',
    displayName: 'Claude',
    harnessDefault: 'claude-code',
    assignable: true,
    liveness: 'offline',
  },
]

export const mockFormationsRunEvents = [
  {
    seq: 1,
    type: 'run_started',
    runId: 'run-playwright-smoke',
    data: { actor: 'playwright' },
  },
  {
    seq: 2,
    type: 'run_blocked',
    runId: 'run-playwright-smoke',
    gateId: 'gate-review',
    data: { reason: 'mocked human gate', resumeAllowed: true },
  },
]

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

async function fulfillSse(route: Route, events: typeof mockFormationsRunEvents) {
  const body = events.map(event => (
    `id: ${event.seq}\nevent: ${event.type}\ndata: ${JSON.stringify(event)}\n\n`
  )).join('')

  await route.fulfill({
    status: 200,
    contentType: 'text/event-stream',
    headers: { 'Cache-Control': 'no-cache' },
    body,
  })
}

export async function mockFormationsApiRoutes(page: Page, options?: {
  board?: typeof mockFormationsBoard
  layout?: typeof mockFormationsLayout
  agents?: typeof mockFormationsAgents
  runEvents?: typeof mockFormationsRunEvents
  runStatus?: {
    runId: string
    status: string
    final: boolean
    boardSlug: string
    missionId: string
    eventCount: number
    resumeAllowed: boolean
    [key: string]: unknown
  }
  escalations?: Array<{
    runId: string
    seq: number
    nodeId?: string
    gateId?: string
    severity: string
    reason: string
    source: string
    trigger: string
    blocks: boolean
  }>
}) {
  const board = options?.board ?? mockFormationsBoard
  const layout = options?.layout ?? mockFormationsLayout
  const agents = options?.agents ?? mockFormationsAgents
  const runEvents = options?.runEvents ?? mockFormationsRunEvents
  const escalations = options?.escalations ?? []
  const runStatus = options?.runStatus ?? {
    runId: 'run-playwright-smoke',
    status: 'blocked',
    final: false,
    boardSlug: board.slug,
    missionId: board.missions[0]?.id ?? 'mission-smoke',
    eventCount: runEvents.length,
    resumeAllowed: true,
  }

  await page.route('**/api/agents**', async route => {
    await fulfillJson(route, { agents, count: agents.length })
  })

  await page.route('**/api/formations/**', async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname

    if (request.method() === 'GET' && path === '/api/formations/gate-profiles') {
      await fulfillJson(route, { profiles: mockCodeGateProfiles })
      return
    }

    if (request.method() === 'GET' && path === '/api/formations/boards') {
      await fulfillJson(route, {
        boards: [{
          id: board.id,
          slug: board.slug,
          title: board.title,
          rev: board.rev,
          etag: board.etag,
        }],
      })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}`) {
      await fulfillJson(route, { board }, { ETag: board.etag })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}/layout`) {
      await fulfillJson(route, { layout }, { ETag: layout.etag })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/boards/${board.slug}/changes`) {
      await fulfillJson(route, {
        signal: {
          board: board.slug,
          changed: false,
          rev: board.rev,
          etag: board.etag,
        },
      })
      return
    }

    if (request.method() === 'POST' && path === '/api/formations/runs') {
      await fulfillJson(route, { runId: runStatus.runId, status: runStatus })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/runs/${runStatus.runId}`) {
      await fulfillJson(route, { status: runStatus })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/runs/${runStatus.runId}/events`) {
      await fulfillJson(route, { events: runEvents })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/runs/${runStatus.runId}/stream`) {
      await fulfillSse(route, runEvents)
      return
    }

    if (request.method() === 'POST' && path === `/api/formations/runs/${runStatus.runId}/resume`) {
      await fulfillJson(route, { status: { ...runStatus, status: 'running', resumeAllowed: false } })
      return
    }

    if (request.method() === 'POST' && path === `/api/formations/runs/${runStatus.runId}/abort`) {
      await fulfillJson(route, { status: { ...runStatus, status: 'canceled', final: true, resumeAllowed: false } })
      return
    }

    if (
      request.method() === 'POST' &&
      path === `/api/formations/runs/${runStatus.runId}/gates/gate-review/verdict`
    ) {
      await fulfillJson(route, { status: { ...runStatus, status: 'succeeded', final: true, resumeAllowed: false } })
      return
    }

    if (request.method() === 'GET' && path === `/api/formations/runs/${runStatus.runId}/escalations`) {
      await fulfillJson(route, { escalations })
      return
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({
        success: false,
        error: { code: 'MOCK_NOT_FOUND', message: `Unhandled Formations mock route: ${request.method()} ${path}` },
      }),
    })
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

export async function mockApiRoutes(page: Page, options?: { sessionsResponse?: SessionsResponse }) {
  // Terminal proxy path only — must not swallow module URLs such as
  // /src/utils/terminalIframe.ts (see the matching route in fixtures.ts).
  await page.route(/\/terminal(\/|\?|$)/, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'text/html',
      body: '<html><body><div class="xterm"><div class="xterm-viewport"><div class="xterm-screen">mock terminal</div></div></div></body></html>',
    })
  })

  await page.route(tmuxAppearancePattern, async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ success: true }),
    })
  })

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

  await page.route(tmuxSessionsPattern, async route => {
    if (route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { name?: string } | null
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ name: body?.name ?? 'shell-test' }),
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

// Beads API mock routes - can be customized per test
export async function mockBeadsApiRoutes(page: Page, options?: {
  projectsResponse?: object
  issuesResponse?: object
  triageResponse?: object
  insightsResponse?: object
}) {
  await page.route('**/api/beads/projects', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(options?.projectsResponse ?? mockBeadsProjects),
    })
  })

  await page.route('**/api/beads/issues**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(options?.issuesResponse ?? mockBeadsIssues),
    })
  })

  await page.route('**/api/beads/triage**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(options?.triageResponse ?? mockBeadsTriage),
    })
  })

  await page.route('**/api/beads/insights**', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(options?.insightsResponse ?? mockBeadsInsights),
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

// Beads API error mock - simulates bv not installed
export async function mockBeadsApiError(page: Page) {
  await page.route('**/api/beads/projects', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockBeadsProjects),
    })
  })

  await page.route('**/api/beads/issues**', async route => {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify(mockBeadsError),
    })
  })

  await page.route('**/api/beads/triage**', async route => {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify(mockBeadsError),
    })
  })

  await page.route('**/api/beads/insights**', async route => {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify(mockBeadsError),
    })
  })
}
