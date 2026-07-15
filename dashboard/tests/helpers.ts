import { Page, Frame, Locator } from '@playwright/test';

/**
 * Shared test helpers for CHROTE dashboard integration tests.
 *
 * Extracted from terminal-sizing.spec.ts, iframe-pool.spec.ts, and
 * dashboard.spec.ts to avoid duplication across test suites.
 *
 * Bead: pol-346b
 */

/** Backend URL used by integration tests that hit the real CHROTE server. */
export const BACKEND_URL = process.env.CHROTE_TEST_URL || 'http://127.0.0.1:8095';

// ---------------------------------------------------------------------------
// StoredStateV2 helpers — mirror the structure in SessionContext.tsx
// ---------------------------------------------------------------------------

interface StoredTerminalWindow {
  id: string;
  boundSessions: string[];
  activeSession: string | null;
  colorIndex: number;
}

interface StoredTerminalWorkspace {
  windows: StoredTerminalWindow[];
  windowCount: number;
}

type WorkspaceId = 'terminal1' | 'terminal2';

interface StoredStateV2 {
  workspaces: Record<WorkspaceId, StoredTerminalWorkspace>;
  sidebarCollapsed: boolean;
  settings: {
    theme?: string;
    fontSize?: number;
    defaultSessionPrefix?: string;
    [key: string]: unknown;
  };
}

const STORAGE_KEY = 'chrote-dashboard-state';

/**
 * Write a StoredStateV2 object into localStorage so the dashboard
 * picks it up on the next page load / reload.
 *
 * The caller provides a partial shape — sensible defaults are merged in.
 *
 * Usage:
 *   await setWorkspaceState(page, {
 *     workspaces: {
 *       terminal1: { windowCount: 1, windows: [{ id: 'terminal1-window-0', boundSessions: ['my-session'], activeSession: 'my-session', colorIndex: 0 }] },
 *       terminal2: { windowCount: 2, windows: [] },
 *     },
 *   });
 */
export async function setWorkspaceState(
  page: Page,
  state: Partial<StoredStateV2>,
): Promise<void> {
  const defaultWindows = (wsId: WorkspaceId, count: number): StoredTerminalWindow[] =>
    Array.from({ length: count }, (_, i) => ({
      id: `${wsId}-window-${i}`,
      boundSessions: [],
      activeSession: null,
      colorIndex: i,
    }));

  const defaults: StoredStateV2 = {
    workspaces: {
      terminal1: { windowCount: 2, windows: defaultWindows('terminal1', 2) },
      terminal2: { windowCount: 2, windows: defaultWindows('terminal2', 2) },
    },
    sidebarCollapsed: false,
    settings: {
      theme: 'matrix',
      fontSize: 14,
      defaultSessionPrefix: 'shell',
    },
  };

  const merged: StoredStateV2 = {
    ...defaults,
    ...state,
    workspaces: {
      terminal1: state.workspaces?.terminal1 ?? defaults.workspaces.terminal1,
      terminal2: state.workspaces?.terminal2 ?? defaults.workspaces.terminal2,
    },
    settings: { ...defaults.settings, ...state.settings },
  };

  await page.evaluate(
    ([key, value]) => localStorage.setItem(key, value),
    [STORAGE_KEY, JSON.stringify(merged)] as const,
  );
}

// ---------------------------------------------------------------------------
// Session creation
// ---------------------------------------------------------------------------

/**
 * Click the "New Session" button inside a specific terminal window and wait
 * for the session tag to appear.  Returns the session name captured from the
 * POST request so the caller can clean it up later.
 *
 * @param windowId  CSS selector or Locator for the target .terminal-window.
 *                  Defaults to the first visible terminal window.
 */
export async function createAndBindSession(
  page: Page,
  windowId?: string | Locator,
): Promise<string> {
  const win: Locator =
    typeof windowId === 'string'
      ? page.locator(windowId)
      : windowId ?? page.locator('.terminal-window').first();

  const createBtn = win.locator('.create-session-btn');

  // Intercept POST to capture name
  let sessionName = '';
  const handler = (req: { method(): string; url(): string; postData(): string | null }) => {
    if (req.method() === 'POST' && req.url().includes('/api/tmux/sessions')) {
      try {
        sessionName = JSON.parse(req.postData() || '{}').name || '';
      } catch { /* ignore */ }
    }
  };
  page.on('request', handler);

  await createBtn.click();
  await win.locator('.session-tag').first().waitFor({ state: 'visible', timeout: 10_000 });

  // Remove listener to avoid leaking across tests
  page.removeListener('request', handler);
  return sessionName;
}

// ---------------------------------------------------------------------------
// Terminal iframe helpers
// ---------------------------------------------------------------------------

/**
 * Return the Playwright Frame object whose URL contains `/terminal/`.
 * Returns null if no such frame exists yet.
 */
export function getTerminalFrame(page: Page): Frame | null {
  return page.frames().find(f => f.url().includes('/terminal/')) ?? null;
}

/**
 * Wait until a `.xterm` element is visible inside the first terminal iframe,
 * then return the Frame for further evaluate() calls.
 */
export async function waitForXtermReady(page: Page, timeout = 10_000): Promise<Frame> {
  const frameLocator = page.locator('.terminal-window-body iframe').first().contentFrame();
  await frameLocator.locator('.xterm').waitFor({ state: 'visible', timeout });
  const frame = getTerminalFrame(page);
  if (!frame) throw new Error('Terminal iframe frame not found after xterm became visible');
  return frame;
}

// ---------------------------------------------------------------------------
// Layout locators
// ---------------------------------------------------------------------------

/**
 * Returns the first visible `.terminal-area-controls` locator
 * (the bar with layout 1/2/3/4 buttons).
 */
export function layoutControls(page: Page): Locator {
  return page.locator('.terminal-area-controls').first();
}

/**
 * Returns the `.terminal-grid` scoped to the terminal1 workspace.
 */
export function visibleArea(page: Page): Locator {
  return page.locator('.terminal-grid[data-workspace="terminal1"]');
}

// ---------------------------------------------------------------------------
// Drag-and-drop (dnd-kit compatible)
// ---------------------------------------------------------------------------

/**
 * Perform a pointer-based drag-and-drop that satisfies dnd-kit's 8 px
 * activation distance.  Accepts CSS selectors.
 */
export async function dragAndDrop(
  page: Page,
  sourceSelector: string,
  targetSelector: string,
): Promise<void> {
  const source = page.locator(sourceSelector).first();
  const target = page.locator(targetSelector).first();

  const sourceBox = await source.boundingBox();
  const targetBox = await target.boundingBox();

  if (!sourceBox || !targetBox) {
    throw new Error('Could not find source or target element for drag-and-drop');
  }

  const startX = sourceBox.x + sourceBox.width / 2;
  const startY = sourceBox.y + sourceBox.height / 2;
  const endX = targetBox.x + targetBox.width / 2;
  const endY = targetBox.y + targetBox.height / 2;

  await page.mouse.move(startX, startY);
  await page.mouse.down();
  // Exceed dnd-kit's 8 px distance threshold
  await page.mouse.move(startX + 10, startY + 10, { steps: 5 });
  await page.mouse.move(endX, endY, { steps: 10 });
  await page.waitForTimeout(100);
  await page.mouse.up();
  await page.waitForTimeout(100);
}

// ---------------------------------------------------------------------------
// Session cleanup
// ---------------------------------------------------------------------------

/**
 * Delete sessions created during a test.  Intended for use in afterEach().
 *
 * Usage:
 *   const sessions: string[] = [];
 *   test.afterEach(({ request }) => cleanupSessions(request, sessions));
 */
export async function cleanupSessions(
  request: { delete(url: string): Promise<unknown> },
  sessionNames: string[],
  baseUrl = BACKEND_URL,
): Promise<void> {
  for (const name of sessionNames) {
    try {
      await request.delete(`${baseUrl}/api/tmux/sessions/${name}`);
    } catch { /* ignore */ }
  }
  sessionNames.length = 0;
}
