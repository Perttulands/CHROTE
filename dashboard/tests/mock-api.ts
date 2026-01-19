import { Page } from '@playwright/test'

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
}

export async function mockApiRoutes(page: Page) {
  await page.route('**/api/tmux/sessions', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(mockSessions),
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
