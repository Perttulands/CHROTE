import { expect, Page } from '@playwright/test';

/**
 * Shared test helpers for the mocked CHROTE browser journeys.
 *
 * Everything here has at least one caller. A helper the specs stopped using is
 * deleted rather than kept warm, so this file stays readable as the suite moves.
 */

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
// Terminal workspace sidecar
// ---------------------------------------------------------------------------

/**
 * Open the Sessions sidecar for tests that exercise session rows. A desktop
 * panel opens in the rail beside the terminals, so terminal targets stay
 * directly interactive; narrow viewports remain overlay-only.
 */
export async function openSessionsSidecar(page: Page): Promise<void> {
  const trigger = page.getByRole('button', { name: 'Sessions sidecar', exact: true });
  await trigger.waitFor({ state: 'visible' });
  if (await trigger.getAttribute('aria-expanded') !== 'true') await trigger.click();

  await page.locator('.session-panel').waitFor({ state: 'visible' });
}

// ---------------------------------------------------------------------------
// Drag-and-drop (dnd-kit compatible)
// ---------------------------------------------------------------------------

/**
 * Perform a pointer-based drag-and-drop that satisfies dnd-kit's 8 px
 * activation distance. Accepts CSS selectors.
 *
 * The waits are on the drag itself rather than on a clock: the dashboard wears
 * `is-dragging` for exactly as long as dnd-kit holds a drag, so this settles on
 * the real start and the real end instead of hoping 100 ms was enough.
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

  const dashboard = page.locator('.dashboard');

  await page.mouse.move(startX, startY);
  await page.mouse.down();
  // Exceed dnd-kit's 8 px distance threshold
  await page.mouse.move(startX + 10, startY + 10, { steps: 5 });
  await page.mouse.move(endX, endY, { steps: 10 });
  await expect(dashboard).toHaveClass(/is-dragging/);
  await page.mouse.up();
  await expect(dashboard).not.toHaveClass(/is-dragging/);
}
