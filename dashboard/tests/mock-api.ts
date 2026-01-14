import { Page } from '@playwright/test'

// Mock session data for testing
export const mockSessions = {
  sessions: [
    { name: 'hq-mayor', agentName: 'mayor', windows: 1, attached: false, group: 'hq' },
    { name: 'hq-deacon', agentName: 'deacon', windows: 1, attached: true, group: 'hq' },
    { name: 'main', agentName: 'main', windows: 2, attached: false, group: 'main' },
    { name: 'gt-gastown-jack', agentName: 'jack', windows: 1, attached: false, group: 'gt-gastown' },
    { name: 'gt-gastown-joe', agentName: 'joe', windows: 1, attached: false, group: 'gt-gastown' },
    { name: 'gt-gastown-max', agentName: 'max', windows: 1, attached: false, group: 'gt-gastown' },
    { name: 'gt-beads-lizzy', agentName: 'lizzy', windows: 1, attached: false, group: 'gt-beads' },
    { name: 'gt-beads-darcy', agentName: 'darcy', windows: 1, attached: false, group: 'gt-beads' },
  ],
  grouped: {
    'hq': [
      { name: 'hq-mayor', agentName: 'mayor', windows: 1, attached: false, group: 'hq' },
      { name: 'hq-deacon', agentName: 'deacon', windows: 1, attached: true, group: 'hq' },
    ],
    'main': [
      { name: 'main', agentName: 'main', windows: 2, attached: false, group: 'main' },
    ],
    'gt-gastown': [
      { name: 'gt-gastown-jack', agentName: 'jack', windows: 1, attached: false, group: 'gt-gastown' },
      { name: 'gt-gastown-joe', agentName: 'joe', windows: 1, attached: false, group: 'gt-gastown' },
      { name: 'gt-gastown-max', agentName: 'max', windows: 1, attached: false, group: 'gt-gastown' },
    ],
    'gt-beads': [
      { name: 'gt-beads-lizzy', agentName: 'lizzy', windows: 1, attached: false, group: 'gt-beads' },
      { name: 'gt-beads-darcy', agentName: 'darcy', windows: 1, attached: false, group: 'gt-beads' },
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
